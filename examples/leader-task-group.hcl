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
  default = "http://host.docker.internal:4646"
}

variable "docker_extra_hosts" {
  type    = list(string)
  default = ["host.docker.internal:host-gateway"]
}

job "leader-task-group" {
  name        = "leader-task-group"
  datacenters = var.datacenters
  type        = "batch"

  meta = {
    "nomad-pipeline.enabled" = "true"
  }

  group "▶️" {
    task "init" {
      driver = "docker"

      config {
        image = var.image
        args  = ["agent", "init"]

        extra_hosts    = var.docker_extra_hosts
        auth_soft_fail = true
      }

      env {
        NOMAD_ADDR           = var.nomad_addr
        NOMAD_PIPELINE_DEBUG = "true"
      }
    }
  }

  group "leader" {
    count = 0

    meta = {
      "nomad-pipeline.root"   = "true"
      "nomad-pipeline.leader" = "true"
    }

    task "some-process" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        sleep 5

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "some-long-running-process" {
    count = 0

    meta = {
      "nomad-pipeline.root" = "true"
    }

    task "forever-run" {
      driver = "raw_exec"

      config {
        command = "/bin/tail"
        args = ["-f", "/dev/null"]
      }
    }
  }
}
