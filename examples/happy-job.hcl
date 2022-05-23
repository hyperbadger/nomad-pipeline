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

job "happy" {
  name        = "happy-job"
  datacenters = var.datacenters
  type        = "batch"

  group "▶️" {
    count = 1

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

  group "1-normal-task" {
    count = 0

    meta = {
      "nomad-pipeline/root" = "true"
      "nomad-pipeline/next" = "2-multi-task-group"
    }

    task "normal" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "do something"
        sleep 5

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "2-multi-task-group" {
    count = 0

    meta = {
      "nomad-pipeline/next" = "3a-parallel,3b-parallel-i"
    }

    task "first_task" {
      driver = "raw_exec"
      config {
        command = "/bin/echo"
        args = ["first_task"]
      }
    }

    task "second_task" {
      driver = "raw_exec"
      config {
        command = "/bin/echo"
        args = ["second_task"]
      }
    }
  }

  group "3a-parallel" {
    count = 0

    meta = {
      "nomad-pipeline/next" = "4-dependent"
    }

    task "parallel" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        sleep 10

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "3b-parallel-i" {
    count = 0

    meta = {
      "nomad-pipeline/next" = "3b-parallel-ii"
    }

    task "parallel" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        sleep 15

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "3b-parallel-ii" {
    count = 0

    meta = {
      "nomad-pipeline/next" = "4-dependent"
    }

    task "parallel" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        sleep 10

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "4-dependent" {
    count = 0

    meta = {
      # BUG: when whole job is restarted, it will not wait for this task group,
      # 4-dependent will run as soon as 3b-parallel-i finishes
      "nomad-pipeline/dependencies" = "3b-parallel-ii"
    }

    task "dependent" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash
        echo "successfully waited for dependency"
        EOT

        destination = "local/main.sh"
      }
    }
  }
}