provider "aws" {
  region = var.aws_region # Choose your preferred region
}


##### IAM

# Fetch SSO Instances Differently
data "aws_ssoadmin_instances" "identity_center" {}

output "identity_center_arn" {
  value = try(tolist(data.aws_ssoadmin_instances.identity_center.arns)[0], "No Identity Center instance found")
}

output "identity_store_id" {
  value = try(tolist(data.aws_ssoadmin_instances.identity_center.identity_store_ids)[0], "No Identity Store ID found")
}

# Create a Permission Set for QA Users
resource "aws_ssoadmin_permission_set" "qa_user" {
  name             = "QA-User-Access"
  description      = "Permission set for QA users"
  instance_arn     = tolist(data.aws_ssoadmin_instances.identity_center.arns)[0]
  session_duration = "PT1H"

  provisioner "local-exec" {
    command = "sleep 10" # Add a small delay to ensure AWS processes the request
  }

  tags = {
    Environment = "qa"
  }
}

# Attach inline policy directly without separate attachment
resource "aws_ssoadmin_permission_set_inline_policy" "qa_permissions" {
  inline_policy = jsonencode({
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
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "iam:GetAccountSummary",
          "iam:ListUsers"
        ]
        Resource = "*"
      }
    ]
  })
  instance_arn       = tolist(data.aws_ssoadmin_instances.identity_center.arns)[0]
  permission_set_arn = aws_ssoadmin_permission_set.qa_user.arn
}

# Create QA Users group in Identity Store
resource "aws_identitystore_group" "qa_users" {
  identity_store_id = tolist(data.aws_ssoadmin_instances.identity_center.identity_store_ids)[0]
  display_name      = "QA-Users"
  description       = "QA team members"
}

# Assign permission set to the account
resource "aws_ssoadmin_account_assignment" "qa_assignment" {
  instance_arn       = tolist(data.aws_ssoadmin_instances.identity_center.arns)[0]
  permission_set_arn = aws_ssoadmin_permission_set.qa_user.arn

  principal_id   = aws_identitystore_group.qa_users.group_id
  principal_type = "GROUP"

  target_id   = var.aws_account_id
  target_type = "AWS_ACCOUNT"
}

##### IAM


#### Auth0 machine - machine


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

#### Auth0 machine - machine


#### VPC, Routes, networking

# VPC Configuration
resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "${var.project_name}-${var.environment}-vpc"
  }
}

# Internet Gateway
resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "qa-backend-igw"
  }
}

# Subnets
resource "aws_subnet" "public" {
  count             = length(var.availability_zones)
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone = var.availability_zones[count.index]

  map_public_ip_on_launch = true

  tags = {
    Name = "${var.project_name}-${var.environment}-subnet-${count.index}"
  }
}

# Route Table
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name = "qa-public-route-table"
  }
}

# Route Table Association
resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# VPC Endpoints for Secrets Manager
resource "aws_vpc_endpoint" "secretsmanager" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.aws_region}.secretsmanager"
  vpc_endpoint_type = "Interface"

  subnet_ids         = aws_subnet.public[*].id
  security_group_ids = [aws_security_group.vpc_endpoint_sg.id]

  private_dns_enabled = true
}

# Security Group for VPC Endpoints
resource "aws_security_group" "vpc_endpoint_sg" {
  name        = "secretsmanager-endpoint-sg"
  description = "Security group for Secrets Manager VPC Endpoint"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}


#### VPC, Routes, networking


# ECS Task IAM Role Updates
resource "aws_iam_role_policy" "secrets_access" {
  name = "ecs-secrets-access"
  role = aws_iam_role.ecs_task_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "kms:Decrypt"
        ]
        Resource = [
          aws_secretsmanager_secret.auth0_client_secret_secret.arn,
          aws_secretsmanager_secret.database_url_secret.arn
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy" "secrets_access_2" {
  name = "ecs-secrets-access"
  role = aws_iam_role.ecs_execution_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "kms:Decrypt"
        ]
        Resource = [
          aws_secretsmanager_secret.auth0_client_secret_secret.arn,
          aws_secretsmanager_secret.database_url_secret.arn
        ]
      }
    ]
  })
}


# ECS Cluster
resource "aws_ecs_cluster" "qa_backend_cluster" {
  name = "qa-backend-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }
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


##### ALB

# Public ALB
resource "aws_lb" "qa" {
  name               = "qa-backend-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  tags = {
    Environment = "qa"
  }
}

# ALB Security Group
resource "aws_security_group" "alb" {
  name        = "qa-alb-sg"
  description = "Security group for QA ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# HTTP Listener
resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.qa.arn
  port              = "80"
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.qa.arn
  }
}

# Target Group
resource "aws_lb_target_group" "qa" {
  name        = "qa-backend-tg"
  port        = 3000
  protocol    = "HTTP"
  vpc_id      = aws_vpc.main.id
  target_type = "ip"

  health_check {
    path                = "/health"
    healthy_threshold   = 2
    unhealthy_threshold = 10
    timeout             = 5
    interval            = 30
  }
}


##### ALB


# RDS PostgreSQL Database
resource "aws_db_subnet_group" "qa_db_subnet_group" {
  name       = "qa-db-subnet-group"
  subnet_ids = aws_subnet.public[*].id
}

resource "aws_db_instance" "qa_database" {
  identifier           = "qa-backend-db"
  allocated_storage    = 20
  storage_type         = "gp2"
  engine               = "postgres"
  engine_version       = "16.1"
  instance_class       = "db.t3.micro"
  db_name              = "myapp"
  username             = var.db_username
  password             = var.db_password
  parameter_group_name = "default.postgres16"
  skip_final_snapshot  = true

  db_subnet_group_name   = aws_db_subnet_group.qa_db_subnet_group.name
  vpc_security_group_ids = [aws_security_group.ecs_tasks.id]
}

# ECS Service
resource "aws_ecs_service" "backend_service" {
  name            = "qa-backend-service"
  cluster         = aws_ecs_cluster.qa_backend_cluster.id
  task_definition = aws_ecs_task_definition.backend_task.arn
  launch_type     = "FARGATE"

  desired_count = 1

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.qa.arn
    container_name   = "backend"
    container_port   = 3000
  }
}

# Task Definition
resource "aws_ecs_task_definition" "backend_task" {
  family                   = "qa-backend-task"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution_role.arn
  task_role_arn            = aws_iam_role.ecs_task_role.arn

  container_definitions = jsonencode([{
    name  = "backend"
    image = "${aws_ecr_repository.main.repository_url}:latest"
    portMappings = [{
      containerPort = 3000
      hostPort      = 3000
    }]
    environment = [
      { name = "ENV", value = "qa" },
      { name = "DATABASE_URL", value = "postgres://${var.db_username}:${var.db_password}@${aws_db_instance.qa_database.endpoint}/myapp" },
      { name = "AUTH0_DOMAIN", value = var.auth0_domain },
      { name = "AUTH0_CLIENT_ID", value = var.auth0_client_id },
      { name = "AUTH0_CALLBACK_URL", value = "http://${aws_lb.qa.dns_name}/callback" }

    ]
    # Secrets configuration
    secrets = [
      {
        name      = "AUTH0_CLIENT_SECRET"
        valueFrom = "${aws_secretsmanager_secret.auth0_client_secret_secret.arn}:AUTH0_CLIENT_SECRET::"
      },
      {
        name      = "DB_CREDENTIALS"
        valueFrom = "${aws_secretsmanager_secret.database_url_secret.arn}:DB_CREDENTIALS::"
      }
    ]
  }])
}

# Secrets Management
resource "aws_secretsmanager_secret" "auth0_client_secret_secret" {
  name = "qa-auth0-client-secret-sm"
}

resource "aws_secretsmanager_secret" "database_url_secret" {
  name = "qa-database-url-sm"
}

# Secret version for Auth0 Client Secret
resource "aws_secretsmanager_secret_version" "auth0_client_secret" {
  secret_id = aws_secretsmanager_secret.auth0_client_secret_secret.id
  secret_string = jsonencode({
    AUTH0_CLIENT_SECRET = var.auth0_client_secret
  })
}

# Secret version for Database Credentials
resource "aws_secretsmanager_secret_version" "database_credentials" {
  secret_id = aws_secretsmanager_secret.database_url_secret.id
  secret_string = jsonencode({
    DB_CREDENTIALS = var.db_password # or whatever credential you want to store
  })
}

# IAM Roles for ECS
resource "aws_iam_role" "ecs_execution_role" {
  name = "qa-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_execution_role_policy" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
  role       = aws_iam_role.ecs_execution_role.name
}

resource "aws_iam_role" "ecs_task_role" {
  name = "qa-ecs-task-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })
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

output "qa_url" {
  value = "http://${aws_lb.qa.dns_name}"
}
