# Contributing

## Local development workflow

## Using `localstack` for developing and testing features with AWS dependencies

Some features depend on AWS services to work. An example of this is using triggers for listening to S3 bucket events to run jobs. To develop and test these features, it is useful to have these services available locally, and `localstack` provides this.

To get `localstack` running, you will first need a local working Nomad environment (see previous section). Once that is setup, it is as simple as running `nomad job run -var examples/localstack.hcl`.

To talk to `localstack` you can use all your standard AWS clients, however the easiest way to get started is using `awslocal`. You can install the `awslocal` CLI by running `pip install awscli-local`. You will also require the AWS CLI package, for v1 `pip install awscli` or for v2 follow [the offical guide](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).

Once `awslocal` is installed, you can test `localstack` is up and running: `awslocal sts get-caller-identity`. You should see the following:

```json
{
    "UserId": "AKIAIOSFODNN7EXAMPLE",
    "Account": "000000000000",
    "Arn": "arn:aws:iam::000000000000:root"
}
```

For more information on `localstack`, see [their docs](https://docs.localstack.cloud/overview/).
