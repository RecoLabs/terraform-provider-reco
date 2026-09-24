terraform {
  required_providers {
    reco = {
      source  = "recolabs/reco"
      version = "~> 0.1"
    }
  }
}

provider "reco" {
  api_key  = var.reco_api_key
  base_url = var.reco_base_url
}

variable "reco_api_key" {
  type      = string
  sensitive = true
}

variable "reco_base_url" {
  type = string
}
