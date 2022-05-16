# Nomad Pipeline

Nomad is great for batch jobs, however in its current state, you can't have job dependecies which is required when running pipeline style workload. The inspiration for this project came from the [`nomad-dag-hack`](https://github.com/cgbaker/nomad-dag-hack) project and the accompanying [blog post](https://www.cgbaker.net/2020/12/hacking-nomad-job-dependencies/).

![](examples/happy-job.gif)

## How to get started?

It's just 2 steps...

**Inject the 'init' task group**

The 'init' task will look at all the meta-tags setup in the next step and inject lifecycle task hooks into the task groups. The hooks are responsible for starting the next task group after the current one finishes.

```hcl
group "▶️" {
  count = 1

  task "init" {
    driver = "docker"

    config {
      image = "ghcr.io/hyperbadger/nomad-pipeline:main"
      args  = ["-init"]
    }

    env {
      NOMAD_ADDR = var.nomad_addr
    }
  }
}
```

**Annotate task groups with meta-tags**

```hcl
group "1-first-task-group" {
  count = 0  # <-- Important! nomad-pipeline will control the count

  meta = {
    "nomad-pipeline/root" = "true"  # <-- Indicates the starting task group
    "nomad-pipeline/next" = "2-second-task-group"
  }

  ...
}

group "2-second-task-group" {
  count = 0

  ...
}
```

## How to run examples?

**Requirements**

- Docker (with default `bridge` network)
- Nomad
- jq

**Steps**

1. Find your Docker `bridge` network gateway IP - `export DOCKER_GATEWAY_IP=$(docker network inspect bridge | jq -r ".[].IPAM.Config[].Gateway")`
1. Start Nomad in dev mode - `nomad agent -dev -bind "${DOCKER_GATEWAY_IP}"`
1. Ensure Nomad has started by visiting `echo "http://${DOCKER_GATEWAY_IP}:4646"`
1. Set `NOMAD_ADDR` for the Nomad CLI to access Nomad - `export NOMAD_ADDR="http://${DOCKER_GATEWAY_IP}:4646"`
1. Ensure Nomad CLI works - `nomad server members`
1. Run any job in the examples/ directory - `nomad job run examples/happy-job.hcl`

## Other features

**Run tasks in parallel**

To support running tasks in parallel and having a final task that runs at the end of all parallel tasks (eg. fan-out fan-in pattern), you can use the `nomad-pipeline/dependencies` tag.

```mermaid
graph TD;
    A-->B;
    A-->C;
    B-->D;
    C-->E;
    D-->E;
```

In the above case, the E task should look like the following, this will ensure that C and D run before E runs, even if C and D finish at different times.

```hcl
group "E" {
  count = 0

  meta = {
    "nomad-pipeline/dependencies" = "C, D"
  }

  ...
}
```

See [`dependencies.hcl`](examples/dependencies.hcl) for a more complete example.

**Dynamic tasks**

Dynamic tasks allows you to have a task that outputs more tasks 🤯. These tasks are then run as part of the job. This can open up the possibility to create some powerful pipelines. An example use case is for creating periodic splits of a longer task, if you have a task that processes 5 hours of some data, you could split the task into 5x 1 hour tasks and run them in parallel. This can be achieved by having an initial task that outputs the 5 split tasks as an output.

To use dynamic tasks, set the `nomad-pipeline/dynamic-tasks` tag to a path/glob of where the task JSON's will be outputted. This path should be relative to [`NOMAD_ALLOC_DIR`](https://www.nomadproject.io/docs/runtime/environment#alloc).

In the following example, the 1-generate-tasks first runs and outputs the 2-echo-hey task group which then gets launched after 1-generate-tasks finishes.

```hcl
group "1-generate-tasks" {
  count = 0

  meta = {
    "nomad-pipeline/root"          = "true"
    "nomad-pipeline/dynamic-tasks" = "tasks/*"
  }

  task "generate-tasks" {
    driver = "raw_exec"

    config {
      command  = "/bin/echo"
      args     = ["generated tasks"]
    }

    template {
      data = <<-EOT
      [{
        "Name": "2-echo-hey",
        "Count": 0,
        "Meta": {
          "nomad-pipeline/root": "true"
        },
        "Tasks": [{
          "Name": "echo",
          "Driver": "raw_exec",
          "Config": { "command": "/bin/echo", "args": [ "hey" ] }
        }]
      }]
      EOT

      destination = "${NOMAD_ALLOC_DIR}/tasks/echo_hey.json"
    }
  }

  ...
}
```

See [`dynamic-job.hcl`](examples/dynamic-job.hcl) for a more complete example.
