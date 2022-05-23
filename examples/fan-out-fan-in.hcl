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

job "fan-out-fan-in" {
  name        = "fan-out-fan-in"
  datacenters = var.datacenters
  type        = "batch"

  group "▶️" {
    task "init" {
      driver = "docker"

      config {
        image = var.image
        args  = ["-init"]

        extra_hosts    = var.docker_extra_hosts
        auth_soft_fail = true
      }

      env {
        NOMAD_ADDR           = var.nomad_addr
        NOMAD_PIPELINE_DEBUG = "true"
      }
    }
  }

  group "1-submit-tasks" {
    count = 0

    meta = {
      "nomad-pipeline/root" = "true"
      "nomad-pipeline/next" = "2-do-work"
    }

    task "submit" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        sleep 5
        echo "alot of work" > queue

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "2-do-work" {
    count = 0

    meta = {
      "nomad-pipeline/count" = "5"
      "nomad-pipeline/next"  = "3-process-output"
    }

    scaling {
      enabled = true
      max     = 10
    }

    task "work" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "pick things off queue and do work"
        # sleep 10
        sleep $((5 + RANDOM % 20));

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "3-process-output" {
    count = 0

    meta = {
      "nomad-pipeline/dependencies" = "2-do-work"
    }

    task "process" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash
        echo "process output of work"
        EOT

        destination = "local/main.sh"
      }
    }
  }
}
