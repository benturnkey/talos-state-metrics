module "vpc" {
  source = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "${var.cluster_name}-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["${var.region}a", "${var.region}b", "${var.region}c"]
  public_subnets  = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]

  enable_dns_hostnames = true
  enable_dns_support   = true

  public_subnet_tags = {
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/elb"                      = "1"
  }
}

resource "aws_security_group" "talos_worker" {
  name        = "${var.cluster_name}-worker-sg"
  description = "Security group for Talos workers"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

module "talos" {
  source = "../../../terraform-aws-talos"

  name = var.cluster_name

  vpc_id               = module.vpc.vpc_id
  controlplane_subnets = [module.vpc.public_subnets[0]] # 1 node CP for cost saving
  instance_type        = "t3.small"
  controlplane_instances = 1

  pod_subnets     = ["10.244.0.0/16"]
  service_subnets = ["10.96.0.0/12"]

  # These would be generated and passed in a real run
  talos_secrets = var.talos_secrets
}

module "talos_workers" {
  source = "../../../terraform-aws-talos/modules/worker"

  name         = "worker"
  cluster_name = module.talos.name

  min_size = 3
  max_size = 3

  vpc_id             = module.vpc.vpc_id
  worker_subnets     = module.vpc.public_subnets
  security_group_ids = [aws_security_group.talos_worker.id]

  allowed_instance_types = ["t3.medium"]
  enable_controlplane_rules = true
  
  # Inherit from control plane module
  talos_secrets                  = var.talos_secrets
  kubernetes_api_server_endpoint = module.talos.kubernetes_api_server_endpoint
  controlplane_aws_security_group_id = module.talos.controlplane_security_group_id
  
  pod_subnets     = module.talos.pod_subnets
  service_subnets = module.talos.service_subnets
}

variable "talos_secrets" {
  type = any
  sensitive = true
}

output "talosconfig" {
  value = module.talos.talosconfig
  sensitive = true
}

output "kubernetes_api_server_endpoint" {
  value = module.talos.kubernetes_api_server_endpoint
}
