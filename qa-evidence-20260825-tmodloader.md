# tModLoader Real QA Evidence

Date: 2026-08-25 (Asia/Shanghai)

## Scope

Temporary QA instance only: `/opt/tr-panel-plugin-v1.5.1-dev.22-qa.1`, panel port `18802`. The formal instance and the older QA instance were not restarted or modified.

## Download and profile

- tModLoader server file: `data/servers/tModLoader/tModLoader.dll`, `21864448` bytes.
- `MagicStorage.tmod`: `1660730` bytes, Workshop ID `2563309347`.
- `SerousCommonLib.tmod`: `87794` bytes, Workshop ID `2908170107`.
- Download queue was empty before the room test.
- Temporary mod profile creation returned success and enabled both mods.

## Room lifecycle

- Precise run created temporary room `#7`, server type `tmodloader`, port `10995`, world `qa-tmod-magic-precise.twld`.
- Start API returned `{"success":true,"message":"房间启动成功"}`.
- Room API reached `status=running`, PID `544929`.
- The process was running as `dotnet .../tModLoader.dll -server` and `ss` showed `0.0.0.0:10995` listening.
- `Mods/enabled.json` contained `MagicStorage` and `SerousCommonLib`.
- Logs showed both mods being sandboxed, added, configured, and finalized:
  - `Magic Storage v0.7.0.11`
  - `absoluteAquarian Utilities (SerousCommonLib) v1.0.6.2`
  - `tModLoader v2026.6.3.6`
- World generation completed and the log reached `Listening on port 10995`.

## Console and WebSocket

- `POST /api/rooms/7/command` with `version` returned success.
- The room log contained the tModLoader version and both mod initialization entries.
- A raw WebSocket client completed `HTTP/1.1 101 Switching Protocols` against `/api/ws/rooms/7/logs`.
- The first frames included `type=connected`, the historical-log marker, and log frames.

## Cleanup

- `POST /api/rooms/7/stop` returned success.
- `DELETE /api/rooms/7` returned success.
- `DELETE /api/modconfig/profiles/3` returned success.
- The temporary room and profile were removed; no temporary tModLoader room was left running.
