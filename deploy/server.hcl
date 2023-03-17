variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
}

variable "image" {
  type    = string
  default = "ghcr.io/hyperbadger/nomad-pipeline:main"
}

variable "nomad_addr" {
  type    = string
  default = "http://localhost:4646"
}

variable "extra_env" {
  type    = map(string)
  default = {}
}

variable "service_provider" {
  type    = string
  default = "nomad"
}

variable "triggers" {
  type = list(
    object({
      job_id  = string
      type    = string
      trigger = map(string)
    })
  )
  default = []
}

job "nomad-pipeline" {
  datacenters = var.datacenters

  type = "service"

  group "server" {

    network {
      port "http" {
        static = var.nomad_addr == "http://localhost:4646" ? 4656 : null
        to     = 4656
      }
    }

    service {
      name     = "nomad-pipeline-server"
      port     = "http"
      provider = var.service_provider
    }

    task "serve" {
      driver = "docker"

      config {
        image = var.image
        args  = [
          "server",
          "--addr", "0.0.0.0:${NOMAD_PORT_http}",
          "--triggers-file", "local/triggers.yaml"
        ]

        ports = ["http"]

        network_mode = var.nomad_addr == "http://localhost:4646" ? "host" : null
      }

      env = merge(
        {
          NOMAD_ADDR = var.nomad_addr
        },
        var.extra_env
      )

      template {
        data        = yamlencode(var.triggers)
        destination = "local/triggers.yaml"
      }

      resources {
        cpu    = 2500
        memory = 512
      }
    }
  }
}