# General Project Variables
variable "aws_account_id" {
  description = "AWS Account ID"
  type        = string
}

variable "project_name" {
  description = "Name of the project"
  type        = string
  default     = "backend-service"
}

variable "environment" {
  description = "Deployment environment (dev/qa/staging/prod)"
  type        = string
  default     = "qa"
}

variable "db_username" {
  description = "db_user"
  type        = string
  default     = "postgres"
}

variable "db_password" {
  description = "db_pass"
  type        = string
}

# AWS Region Configuration
variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-east-1"
}

# VPC Configuration
variable "vpc_cidr" {
  description = "CIDR block for VPC"
  type        = string
  default     = "10.0.0.0/16"
}

# GitHub Configuration
variable "github_org" {
  description = "GitHub Organization Name"
  type        = string
}

variable "github_repo" {
  description = "GitHub Repository Name"
  type        = string
}

# ECS Configuration
variable "ecs_task_cpu" {
  description = "CPU units for ECS task"
  type        = number
  default     = 256
}

variable "ecs_task_memory" {
  description = "Memory for ECS task (in MiB)"
  type        = number
  default     = 512
}

# Application Configuration
variable "app_port" {
  description = "Port the application runs on"
  type        = number
  default     = 3000
}

variable "auth0_domain" {
  description = "domain"
  type        = string
}

variable "auth0_client_id" {
  description = "client id"
  type        = string
}

# Subnet Configuration
variable "availability_zones" {
  description = "List of availability zones"
  type        = list(string)
  default     = ["us-east-1a", "us-east-1b"]
}
