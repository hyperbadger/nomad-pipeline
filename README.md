# Nomad Pipeline

Nomad is great for batch jobs, however in its current state, you can't have job dependecies which is required when running pipeline style workload. The inspiration for this project came from the [`nomad-dag-hack`](https://github.com/cgbaker/nomad-dag-hack) project and the accompanying [blog post](https://www.cgbaker.net/2020/12/hacking-nomad-job-dependencies/).

![](examples/happy-job.gif)

## How to get started?

It's just 2 steps...

**Inject the 'init' task group**

The 'init' task will look at all the meta-tags setup in the next step and inject lifecycle task hooks into the task groups. The hooks are responsible for starting the next task group after the current one finishes.

```hcl
variable "nomad_addr" {
  type    = string
}

job "example-job" {

  meta = {
    "nomad-pipeline.enabled" = "true"
  }

  group "▶️" {
    count = 1

    task "init" {
      driver = "docker"

      config {
        image = "ghcr.io/hyperbadger/nomad-pipeline:main"
        args  = ["agent", "init"]
      }

      env {
        NOMAD_ADDR = var.nomad_addr
      }
    }
  }

  ...
}
```

**Annotate task groups with meta-tags**

```hcl
job "example-job" {

  ...

  group "1-first-task-group" {
    count = 0  # <-- Important! nomad-pipeline will control the count

    meta = {
      "nomad-pipeline.root" = "true"  # <-- Indicates the starting task group
      "nomad-pipeline.next" = "2-second-task-group"
    }

    ...
  }

  group "2-second-task-group" {
    count = 0

    ...
  }

  ...
}

```

## How to run examples?

**Requirements**

- Docker
- Nomad

> To make it simple to run the examples, we are running using `network_mode="host"`, this means that the example containers will be running in the host networking namespace and have access to localhost - where Nomad should be listening on.

**Scenario A: Running Nomad agent in dev mode**

1. Start Nomad in dev mode - `nomad agent -dev` - tip: if you want to access to Nomad from other machines or you don't have direct access to `localhost`, you can run with `-bind 0.0.0.0`
1. Ensure Nomad has started by visiting `echo "http://localhost:4646"` or replace `localhost` with the address of where Nomad is running
1. Set `NOMAD_ADDR` for the Nomad CLI to access Nomad - `export NOMAD_ADDR="http://localhost:4646"` or replace `localhost` with the address of where Nomad is running
1. Ensure Nomad CLI works - `nomad server members`
1. Run any job in the examples/ directory - `nomad job run examples/happy-job.hcl`

**Scenario B: Nomad running as a service on localhost**

If installing Nomad using a package manager, it might already setup a service unit for you. On Ubuntu, you can check this by running `sudo systemctl status nomad`.

1. Ensure the `raw_exec` driver is enabled - for simplicity the examples use `raw_exec` tasks. You should have the following in your Nomad config (eg. `/etc/nomad.d/nomad.hcl`):
    ```hcl
    plugin "raw_exec" {
      config {
        enabled = true
      }
    }
    ```
1. Set `NOMAD_ADDR` for the Nomad CLI to access Nomad - `export NOMAD_ADDR="http://localhost:4646"`
1. Ensure Nomad CLI works - `nomad server members`
1. Run any job in the examples/ directory - `nomad job run examples/happy-job.hcl`

**Scenario C: Existing Nomad cluster**

1. Set `NOMAD_ADDR` to your clusters Nomad server, this address should be accessible from containers running on the cluster - `export NOMAD_ADDR="https://nomad.hyperbadger.cloud"`
1. Ensure Nomad CLI works - `nomad server members`
1. Run any job in the examples/ directory - `nomad job run -var "nomad_addr=${NOMAD_ADDR}" examples/happy-job.hcl`

### Bonus: Deploying the server component

Some examples require the server component to be deployed.

1. Ensure the correct `NOMAD_ADDR` is set using the instructions above
1. `nomad job run -var "nomad_addr=${NOMAD_ADDR}" deploy/server.hcl`
1. If your `NOMAD_ADDR` is `http://localhost:4646`, you should be able to test the newly deployed server by running `curl http://localhost:4656/jobs`

## Features

There are many features that work with just the 'init' task. This allows you to get started with minimal changes to your setup and it doesn't have any dependencies on other services. However there are more features which can only be accessed by running a server component a.k.a 'nomad-pipeline server'. Different features are highlighted below, for each feature, a badge is used to indicate if the feature requires just the init task, the server, or both.

**Run tasks in parallel**

![init](https://img.shields.io/badge/init-blue)

***Using dependencies***

To support running tasks in parallel and having a final task that runs at the end of all parallel tasks (eg. fan-out fan-in pattern), you can use the `nomad-pipeline.dependencies` tag.

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
    "nomad-pipeline.dependencies" = "C, D"
  }

  ...
}
```

See [`dependencies.hcl`](examples/dependencies.hcl) for a more complete example.

***Using count***

Another way to implement the fan-out fan-in pattern is to have multiple instances of a task group that can all pick up some work. Without nomad-pipeline, this is quite easy, you just set the [`count` stanza](https://www.nomadproject.io/docs/job-specification/group#count) on the task group. However, when using nomad-pipeline, the control of count is not in your hands. So if you want to set a count greater than 1, you can set the `nomad-pipeline.count` tag.

> 💡 *Tip: The [`count` stanza](https://www.nomadproject.io/docs/job-specification/group#count) doesn't support variable interpolation since the config value is an integer and not a string - currently Nomad only support variable interpolation for string config values. This means that `count` can't be set from a `NOMAD_META_` variable, which is required for setting the `count` dynamically in a parameterized job. Using the `nomad-pipeline.count` tag allows you work around this. All `nomad-pipeline.*` tags interpolates variables, so you can use something like `"nomad-pipeline.count" = "${NOMAD_META_count}"`*

See [`examples/fan-out-fan-in.hcl`](examples/fan-out-fan-in.hcl) for a more complete example.

**Dynamic tasks**

![init](https://img.shields.io/badge/init-blue)

Dynamic tasks allows you to have a task that outputs more tasks 🤯. These tasks are then run as part of the job. This can open up the possibility to create some powerful pipelines. An example use case is for creating periodic splits of a longer task, if you have a task that processes 5 hours of some data, you could split the task into 5x 1 hour tasks and run them in parallel. This can be achieved by having an initial task that outputs the 5 split tasks as an output.

To use dynamic tasks, set the `nomad-pipeline.dynamic-tasks` tag to a path/glob of where the task JSON's will be outputted. This path should be relative to [`NOMAD_ALLOC_DIR`](https://www.nomadproject.io/docs/runtime/environment#alloc).

In the following example, the 1-generate-tasks first runs and outputs the 2-echo-hey task group which then gets launched after 1-generate-tasks finishes.

```hcl
group "1-generate-tasks" {
  count = 0

  meta = {
    "nomad-pipeline.root"          = "true"
    "nomad-pipeline.dynamic-tasks" = "tasks/*"
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
          "nomad-pipeline.root": "true"
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

**Job Level Leader**

![init](https://img.shields.io/badge/init-blue)

Nomad currently allows you to set a [`leader`](https://www.nomadproject.io/docs/job-specification/task#leader) at the task level. This allows you to gracefully shutdown all other tasks in the group when the leader task exits.

Using the `nomad-pipeline.leader` tag, you can get the same functionality at the job level. You can set the tag on a task group, and when that task group completes, all other task groups will be gracefully shutdown.

```hcl
group "leader" {
  count = 0

  meta = {
    "nomad-pipeline.leader" = "true"
  }

  ...
}
```

See [`leader-task-group.hcl`](examples/leader-task-group.hcl) for a more complete example.

**URL Friendly Nomad Environment Variables**

![init](https://img.shields.io/badge/init-blue)

There are many useful [Nomad environment variables](https://www.nomadproject.io/docs/runtime/interpolation#interpreted_env_vars) that can be used at runtime and in config fields that support variable interpolation. However, in some cases, some of these environment variables are not URL friendly - in the case of parameterized jobs, the dispatched job's ID (`NOMAD_JOB_ID`) and name (`NOMAD_JOB_NAME`) will have a `/` in them. URL friendly versions of these variables are required when using them in the [`service` stanza](https://www.nomadproject.io/docs/job-specification/service#name). To allow for this, a URL friendly version of the `NOMAD_JOB_ID` and `NOMAD_JOB_NAME` can be found under `NOMAD_META_JOB_ID_SLUG` and `NOMAD_META_JOB_ID_SLUG` - the inspiration for `_SLUG` came from [Gitlab predefined variables](https://docs.gitlab.com/ee/ci/variables/predefined_variables.html). These meta variables are injected at the job level by the init task of nomad-pipeline, making them available to all the task groups that come after it.

Although this feature was added specifically for use with the [`service` stanza](https://www.nomadproject.io/docs/job-specification/service#name), it could prove useful for other config fields. Note to developer: nomad-pipeline might not be the right vehicle for this feature, however the init task was a convenient place to put this functionality.

**Triggered pipelines**

![server](https://img.shields.io/badge/server-green)

Having pipelines (or jobs) react to events can be a powerful mechanism for creating automated workflows. To start using this feature, it is important to have the server component running. Triggers are configured using a yaml file which is passed into the server using the `--triggers-file` flag. Below is an example of a minimal triggers config file along with a job that would be triggered.

```yaml
- job_id: ffmpeg-transcode  # <-- job id of a parameterized job
  type: s3  # <-- type of trigger, s3 trigger allows to listen to bucket changes
  trigger:
    sqs_url: awssqs://sqs.us-east-2.amazonaws.com/000000000000/videos-to-transcode-queue
    meta_key: video_object_path  # <-- meta key of job to use to pass the path of object in event
```

```hcl
job "ffmpeg-transcode" {

  meta = {
    "video_object_path" = ""  # configured using the `meta_key` option
  }

  parameterized {
    meta_required = ["video_object_path"]
  }

  group "ffmpeg" {

    task "transcode" {
      driver = "docker"

      config {
        image = "hyperbadger/ffmpeg-transcoder"
        args  = [
          "-path",
          "${NOMAD_META_video_object_path}"  # <-- NOMAD_META_video_object_path will be the path to the
        ]                                    #     object that was created eg. incoming/test.mp4
      }
    }

    ...
  }
  ...
}
```

For more detailed examples and documentation on triggers, see [`triggers.md`](docs/triggers.md).
