# Introduction

Tusk Drift enables you to record and replay traces from real traffic. These recorded traces make up
an API test suite, which you can run locally or in CI/CD pipelines.

## Concepts overview

A **trace** represents the full path of an inbound request in your application.  

A **span** represents a unit of work or operation in a trace. We capture all outbound requests made from your service (e.g., database calls, HTTP requests) in the path of a trace.

Further reading: <https://opentelemetry.io/docs/concepts/signals/traces/>

## Setup

Setup involves creating a `.tusk/config.yaml` file in the directory of the service you wish to test.
This lets Tusk know how to start your service and wait for it to ready during replay mode.

You can run `tusk init` to start the configuration wizard.

## Test workflow

1. Instrument your app with the Tusk SDK.
1. Once your app is up and ready, send traffic to it.
    - Tusk will record traces in `.tusk/traces`.
    - Shut down your app when you're done.
1. Run `tusk run` to run these traces locally.
1. Set up a workflow to run tests in your CI/CD pipeline.
