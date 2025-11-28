# Tusk Drift Cloud

## Getting started

[Sign up for a Tusk account](https://app.usetusk.ai/app) or run `tusk auth login`. You must also have created a Tusk config file (i.e., complete `tusk init` first).

Then, execute

```bash
tusk cloud-init
```

to start an onboarding wizard that will guide you to:

- Authorize the Tusk app for your code hosting service
- Register your service for Tusk Drift Cloud
- Obtain an API key to use Tusk Drift in CI/CD pipelines

## Run Tusk Drift in CI/CD

### GitHub

- We recommend adding your `TUSK_API_KEY` to your GitHub secrets.
- Refer to an [example GitHub Actions workflow](./github-workflow-example.yml). Adapt this accordingly for your service.

## Troubleshooting

Running into issues? Contact us at <support@usetusk.ai> and we'll be right with you.
