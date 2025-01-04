provider "aws" {
  region = var.aws_region # Choose your preferred region
}

# Fetch SSO Instances Differently
data "aws_ssoadmin_instances" "default" {}

# # Create Permission Set
# resource "aws_ssoadmin_permission_set" "qa_read_only" {
#   # Use the first SSO instance
#   instance_arn = tolist(data.aws_ssoadmin_instances.default.arns)[0]
#
#   name = "QA-ReadOnly-Access"
#   description = "Read-only access for QA team"
#
#   # Session duration
#   session_duration = "PT2H"  # 2 hours
# }
#
# # Attach Read-Only Policy
# resource "aws_ssoadmin_managed_policy_attachment" "read_only" {
#   instance_arn       = tolist(data.aw.default.arns)[0]
#   permission_set_arn = aws_ssoadmin_permission_set.qa_read_only.arn
#   managed_policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
# }

# Direct IAM Role
resource "aws_iam_role" "qa_read_only" {
  name = "QA-ReadOnly-Access"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${var.aws_account_id}:root"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "qa_read_only" {
  role       = aws_iam_role.qa_read_only.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}


resource "aws_iam_role" "qa_ecs_access" {
  name = "QA-ECS-Access"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${var.aws_account_id}:root"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "qa_ecs_access_policy" {
  name = "QA-ECS-Access-Policy"
  role = aws_iam_role.qa_ecs_access.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecs:DescribeServices",
          "ecs:ListServices",
          "ecs:DescribeClusters",
          "ecs:ListClusters",
          "ecs:ListTasks",
          "ecs:DescribeTasks",
          "logs:GetLogEvents",
          "logs:DescribeLogGroups"
        ]
        Resource = "*"
      }
    ]
  })
}


# Get current account details
data "aws_caller_identity" "current" {}


# Update Auth0 Configuration to use Actions
resource "auth0_action" "qa_access_control" {
  name    = "QA Access Control"
  runtime = "node16"
  deploy  = true

  code = <<-EOF
  exports.onExecutePostLogin = async (event, api) => {
    const qaEmails = ['external-qa@lariatlabs.dev'];

    if (!qaEmails.includes(event.user.email)) {
      api.access.deny('Access denied');
    }
  };
  EOF

  supported_triggers {
    id      = "post-login"
    version = "v3"
  }
}

# Create an Action Binding
resource "auth0_trigger_actions" "login_flow" {
  trigger = "post-login"

  actions {
    id           = auth0_action.qa_access_control.id
    display_name = auth0_action.qa_access_control.name
  }
}

# VPC Configuration
resource "aws_vpc" "main" {
  cidr_block = var.vpc_cidr

  tags = {
    Name = "${var.project_name}-${var.environment}-vpc"
  }
}

# Subnets
resource "aws_subnet" "public" {
  count             = length(var.availability_zones)
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone = var.availability_zones[count.index]

  tags = {
    Name = "${var.project_name}-${var.environment}-subnet-${count.index}"
  }
}

# ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = "qa-backend-cluster"
}

# ECR Repository
resource "aws_ecr_repository" "main" {
  name = "qa-backend-repo"
}

# Security Group
resource "aws_security_group" "ecs_tasks" {
  name        = "qa-backend-sg"
  description = "Allow inbound traffic"
  vpc_id      = aws_vpc.main.id

  ingress {
    protocol    = "tcp"
    from_port   = var.app_port
    to_port     = var.app_port
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    protocol    = "-1"
    from_port   = 0
    to_port     = 0
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# ECS Task Definition
resource "aws_ecs_task_definition" "main" {
  family                   = "qa-backend-task"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution_role.arn

  container_definitions = jsonencode([{
    name  = "qa-backend-container"
    image = "${aws_ecr_repository.main.repository_url}:latest"
    portMappings = [{
      containerPort = 3000
      hostPort      = 3000
    }]
    environment = [
      {
        name  = "ENV"
        value = "qa"
      }
    ]
  }])
}

# IAM Roles for ECS
resource "aws_iam_role" "ecs_execution_role" {
  name = "qa-backend-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      },
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = aws_iam_role.github_actions_role.arn
        }
      }
    ]
  })
}

# Deployment Script
resource "null_resource" "docker_packaging" {
  provisioner "local-exec" {
    command = <<EOF
      aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin ${aws_ecr_repository.main.repository_url}
      docker build -t ${aws_ecr_repository.main.repository_url}:latest .
      docker push ${aws_ecr_repository.main.repository_url}:latest
    EOF
  }

  depends_on = [aws_ecr_repository.main]
}

data "aws_iam_openid_connect_provider" "github_actions" {
  url = "https://token.actions.githubusercontent.com"
}

# IAM Role for GitHub Actions
resource "aws_iam_role" "github_actions_role" {
  name = "github-actions-deployment-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRoleWithWebIdentity"
        Effect = "Allow"
        Principal = {
          #Federated = aws_iam_openid_connect_provider.github_actions.arn
          Federated = data.aws_iam_openid_connect_provider.github_actions.arn
        }
        Condition = {
          StringEquals = {
            "token.actions.githubusercontent.com:aud" : "sts.amazonaws.com"
          }
          StringLike = {
            "token.actions.githubusercontent.com:sub" : "repo:${var.github_org}/${var.github_repo}:*"
          }
        }
      }
    ]
  })
}

# Deployment Policy
resource "aws_iam_role_policy" "github_actions_policy" {
  name = "github-actions-deployment-policy"
  role = aws_iam_role.github_actions_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:BatchCheckLayerAvailability",
          "ecr:PutImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload",
          "ecr:GetAuthorizationToken",
          "ecs:UpdateService",
          "ecs:DescribeServices",
          "ecs:DescribeTaskDefinition",
          "ecs:RegisterTaskDefinition"
        ]
        Resource = "*"
      }
    ]
  })
}

# Policy for ECS Deployment
resource "aws_iam_role_policy" "github_actions_ecs_policy" {
  name = "github-actions-ecs-deployment-policy"
  role = aws_iam_role.github_actions_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecs:UpdateService",
          "ecs:DescribeServices",
          "ecs:DescribeTaskDefinition",
          "ecs:RegisterTaskDefinition",
          "iam:PassRole"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
          "ecr:BatchCheckLayerAvailability",
          "ecr:PutImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload"
        ]
        Resource = aws_ecr_repository.main.arn
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:GetAuthorizationToken"
        ]
        Resource = "*"
      }
    ]
  })
}

# CloudTrail for comprehensive logging
# S3 Bucket for CloudTrail Logs
resource "aws_s3_bucket" "logging_bucket" {
  bucket        = "qa-access-logs-${data.aws_caller_identity.current.account_id}"
  force_destroy = true
}

# S3 Bucket Public Access Block
resource "aws_s3_bucket_public_access_block" "logging_bucket_access" {
  bucket = aws_s3_bucket.logging_bucket.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# S3 Bucket Policy for CloudTrail
resource "aws_s3_bucket_policy" "cloudtrail_bucket_policy" {
  bucket = aws_s3_bucket.logging_bucket.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AWSCloudTrailAclCheck"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:GetBucketAcl"
        Resource = aws_s3_bucket.logging_bucket.arn
      },
      {
        Sid    = "AWSCloudTrailWrite"
        Effect = "Allow"
        Principal = {
          Service = "cloudtrail.amazonaws.com"
        }
        Action   = "s3:PutObject"
        Resource = "${aws_s3_bucket.logging_bucket.arn}/*"
        Condition = {
          StringEquals = {
            "s3:x-amz-acl" = "bucket-owner-full-control"
          }
        }
      }
    ]
  })
}

# CloudTrail Configuration
resource "aws_cloudtrail" "qa_access_trail" {
  name                          = "qa-access-trail"
  s3_bucket_name                = aws_s3_bucket.logging_bucket.id
  include_global_service_events = true

  event_selector {
    read_write_type           = "All"
    include_management_events = true
  }

  # Ensure the bucket policy is created before the trail
  depends_on = [aws_s3_bucket_policy.cloudtrail_bucket_policy]
}

# Optional: Enable encryption for the S3 bucket
resource "aws_s3_bucket_server_side_encryption_configuration" "logging_bucket_encryption" {
  bucket = aws_s3_bucket.logging_bucket.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Outputs
output "github_actions_role_arn" {
  value = aws_iam_role.github_actions_role.arn
}


