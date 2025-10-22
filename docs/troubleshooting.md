# Troubleshooting

## Connection Issues

- **SDK Connection Failure**: Ensure your service uses the Tusk Drift SDK and is started by the CLI (so it sees `TUSK_MOCK_SOCKET`).
- **Docker Services**: If your service starts using a Docker container, refer to [Docker configuration](docs/configuration.md#docker-support).
- **TCP Port Issues**: If using TCP to connect with SDK (usually for Docker setups), ensure `service.communication.tcp_port` is not in use.

## Configuration Issues

- **Port Already in Use**: The CLI will block if `service.port` is already taken.
- **Readiness Check**: If `service.readiness_check.command` is omitted, the CLI waits ~10s before replay.
- **Cloud Mode Setup**: Ensure `service.id`, `tusk_api.url`, and `TUSK_API_KEY` or `tusk login` are set.

## Replay Issues

- **No Mock Found**: Check suite spans availability and matching rules; ensure traces exist for the trace being replayed.
- **Environment Mismatch**: If you can record traces successfully but unable to replay them, check if you are running `tusk run` in an environment similar to what you recorded the traces in. For example, for Node.js services, a common issue could be a difference in Node versions.

## Getting Help

If you have any questions, feel free to open an issue or reach us at [support@usetusk.ai](support@usetusk.ai).
