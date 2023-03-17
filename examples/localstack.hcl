variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
}

variable "image" {
  type    = string
  default = "localstack/localstack"
}

job "localstack" {
  datacenters = var.datacenters

  type = "service"

  group "localstack" {

    network {
      mode = "host"

      port "gateway" {
        static = 4566
        to     = 4566
      }
    }

    task "serve" {
      driver = "docker"

      config {
        image = var.image
        
        network_mode = "host"
        ports        = ["gateway"]
      }

      resources {
        cpu    = 2500
        memory = 512
      }
    }
  }
}