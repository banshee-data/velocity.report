# Server settings and connection status

Where VelocityVisualiser finds the two servers it talks to, how it keeps its connection to velocity.report up, and what the status indicator inlaid in the top left of the 3D view says.

- **Code:** [ServerSettingsView.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/ServerSettingsView.swift) (Settings ▸ Servers), [GerardusSettingsView.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/GerardusSettingsView.swift) (its Gerardus sections), [ServerStatusIndicator.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/ServerStatusIndicator.swift) (the indicator), [VelocityReportServer.swift](../../../tools/visualiser-macos/VelocityVisualiser/App/VelocityReportServer.swift) (the URL, its parsing and its preference), [ServerConnection.swift](../../../tools/visualiser-macos/VelocityVisualiser/App/ServerConnection.swift) (status, backoff and Test connection)
- **Tests:** [ServerSettingsTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/ServerSettingsTests.swift), [GerardusSettingsTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/GerardusSettingsTests.swift)

## The two servers

| Server          | What the app uses it for                                                                       | Default                                         | Kept as                                                 |
| --------------- | ---------------------------------------------------------------------------------------------- | ----------------------------------------------- | ------------------------------------------------------- |
| velocity.report | The frame stream the main window draws (gRPC), and runs, labels and replays (HTTP API)         | `http://localhost:8080`, stream on port `50051` | `velocityReport.serverURL`, `velocityReport.streamPort` |
| Gerardus        | Alignment and the street painter; optional, see [gerardus-alignment.md](gerardus-alignment.md) | `127.0.0.1:50061`                               | `alignment.serviceAddress`                              |

An unset preference means the default, so choosing **Use this Mac** removes the keys rather than writing the default into them.

## Settings ▸ Servers

Settings (⌘,) has one tab, **Servers**, with a section for each server and one for the street painter's author. Clicking the status indicator opens it.

### velocity.report

- **Server URL:** the HTTP API's URL. A URL without a scheme is `http`; without a port, `http` means 8080 and `https` 443. A path such as `/velocity` is kept as a prefix for every API call, for a server behind a reverse proxy. An IPv6 literal goes in brackets: `http://[fd7a:115c:a1e0::1]:8080`.
- **Stream port:** the gRPC frame stream's port on the same host (the server's `--lidar-grpc-listen`). Empty means 50051.
- **Validation:** a scheme other than `http` or `https`, a user name, a query or fragment, a bad host name or a port outside 1 to 65535 is refused with the reason under the fields, and nothing changes.
- **Applying:** the fields are a draft. **Apply**, or Enter in either field, saves the server and reconnects once; typing alone changes nothing. Applying the server already in use is not a change, and does not reconnect.
- **Connection:** the live connection's state, the URL and stream address in use, the last error, and the wait before the next retry while one is scheduled.
- **Test connection:** tests what is typed, so a server can be tried before it is applied, with a 3 s timeout on each of two calls made on channels of their own:
  - `GET /api/version` on the HTTP API: reachable with the server's version, git sha and build time, and the round trip in milliseconds.
  - `GetCapabilities` on the stream's port: reachable with the round trip and the sensors it offers.
  - Each reads as reachable, wrong service (something answered that is not velocity.report: an HTTP status other than 200, an answer that is not the version, or gRPC `UNIMPLEMENTED`), unreachable (refused, no route, no such host) or no answer within the timeout.

### Gerardus

As described in [gerardus-alignment.md](gerardus-alignment.md#which-gerardus-server): the address, parsed by `GerardusAddress`, with **Connect** reconnecting at once. **Test connection** asks `GetServerInfo` on a channel of its own, at the typed address when it parses, and reports reachable with the latency, wrong service (gRPC `UNIMPLEMENTED`: a server without Gerardus's `SiteService`, such as velocity.report's stream port), unreachable, or no answer within 3 s. **This Gerardus** shows the version, git sha, API version and whether it serves registered scans. When the test finds the server in use while the app had not, the app takes it up at once.

### Moving the old preference

Before this tab, the HTTP API's address could be set only with `defaults write` as `annotation.serviceURL`, and the stream was always `localhost:50051`. At launch the app moves `annotation.serviceURL` to `velocityReport.serverURL` once, and reads the old key until it has. A value the new parser refuses is left where it was, and the app uses this Mac's server.

Every HTTP client reads the saved URL at each call, so an applied change reaches the run browser, labels, annotation and alignment without a relaunch.

## The status indicator

The top left of the 3D view carries a small panel with a dot and a name for velocity.report and, under it, for Gerardus. A healthy server is its dot and its name alone; one in error adds the salient part of what went wrong (the cause inside a wrapper's parentheses, or the text after a `code:` or `Error:` prefix, at most 48 characters). It is a status indicator, not a control: it cannot connect or disconnect.

| State        | Dot    | velocity.report                                                                     | Gerardus                                      |
| ------------ | ------ | ----------------------------------------------------------------------------------- | --------------------------------------------- |
| Connected    | green  | The visualiser service answered: `GetCapabilities`, or the frame stream accepted    | `GetServerInfo` answered                      |
| Connecting…  | yellow | An attempt is in flight, or a transport exists but the service has not answered yet | A probe is in flight                          |
| Disconnected | grey   | No connection and no error                                                          | Not asked yet                                 |
| Error        | red    | The last attempt failed or the stream dropped, with the salient reason              | Asked with no answer, with the salient reason |

The tooltip gives each server's address, its state, its last error in full and, for velocity.report, when the next retry is due. Clicking the indicator opens Settings ▸ Servers.

## Connecting and retrying

- **At launch** the app connects to the saved velocity.report server, half a second after it starts.
- **While it is unreachable** the app retries with backoff: 1 s after the first failure, then 2, 4, 8 and 16 s, and every 30 s after that. A failed attempt, a stream that ends with an error and a live stream that ends at all each count as a failure. A replay that plays to its end is not a failure: Play restarts it, as before.
- **Once the service answers** the count starts again, so the next drop is retried after 1 s.
- **File ▸ Reconnect (⌘R)** drops whatever connection there is and connects again at once, with the backoff started afresh. Applying a changed URL in Settings does the same.
- **Gerardus** is asked while velocity.report is connected, at once and every 30 seconds until it answers, as before. Connected means the service answered: a retry that only makes a transport does not set Gerardus asking.

The old File ▸ Connect/Disconnect item (⇧⌘C) and the toolbar's Connect/Disconnect button are gone; Reconnect replaces both.
