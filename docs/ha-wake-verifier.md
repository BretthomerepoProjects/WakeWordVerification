# Home Assistant Wake Word Verification

Add the WakeWordVerification GitHub repository in HACS under Custom repositories, with type Integration. Download Wake Word Verification. Configure the following in configuration.yaml and restart Home Assistant. Replace the examples with your Spot 2 details and the STT entity backed by Parakeet.

```yaml
wake_word_verification:
  satellite_ip: "192.0.2.20"
  satellite_entity_id: assist_satellite.spot_2
  satellite_name: "Spot 2"
  phrase: "Alexa"
  stt_engine: stt.parakeet
  language: en-US
  archive_dir: /wake-archive
```

Use an existing private writable archive mount outside the Home Assistant configuration directory. Keep the path out of publicly served directories. The Parakeet connection is configured in its Home Assistant STT provider, such as a Wyoming STT entity backed by Parakeet. No token is provisioned on Spot 2. Use a reserved IP and reach Home Assistant directly, since the listener checks the immediate connection peer and ignores forwarded headers.

## Satellite protocol

POST /api/wake_word_verification/v1/verify with Content-Type application/octet-stream, X-Wake-Event-Id (UUID), X-Wake-Score (0–1), and optionally X-Wake-Clarity (0–1). Send up to two seconds of mono 16 kHz signed 16-bit little-endian PCM. The response contains event_id, approved, and reason. Fail closed on timeout, non-200 status, or an unexpected event ID. Requests must use a fresh ID for each candidate. This release does not include the firmware client.

## Records

The archive contains wake-events.sqlite3 and one WAV per saved request. The wake_events table holds both scores, STT text, a private file URI, audio BLOB and WAV checksum, verdict and reason, timestamp, duration, STT latency, expected phrase, and mapped satellite entity ID and name. Missing clarity is NULL. The file URI is visible to an operator with filesystem access to the archive mount.

Archive writes run in the background with a bounded backlog. Write failures and overload are reported with the event ID in Home Assistant logs. The pilot does not have a durable retry queue or automatic retention. A restart can lose pending writes. Requests rejected before PCM validation have no clip to archive.
