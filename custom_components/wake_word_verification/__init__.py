"""Wake verification for an IP-mapped satellite."""
import asyncio
from contextlib import closing
from datetime import datetime, timezone
import hashlib
import io
from ipaddress import ip_address
import logging
import math
from pathlib import Path
import sqlite3
import time
import unicodedata
from uuid import UUID
import wave

from aiohttp import web
import voluptuous as vol

from homeassistant.components import stt
from homeassistant.components.http import HomeAssistantView
from homeassistant.helpers import config_validation as cv

DOMAIN = "wake_word_verification"
_LOGGER = logging.getLogger(__name__)
MAX_AUDIO_BYTES = 64000

CONFIG_SCHEMA = vol.Schema({
    DOMAIN: vol.Schema({
        vol.Required("satellite_ip"): vol.All(cv.string, vol.Coerce(ip_address)),
        vol.Required("satellite_entity_id"): cv.entity_id,
        vol.Required("satellite_name"): cv.string,
        vol.Required("phrase"): vol.All(cv.string, vol.Length(min=1, max=80)),
        vol.Required("stt_engine"): cv.string,
        vol.Required("language"): cv.string,
        vol.Required("archive_dir"): cv.string,
    }),
}, extra=vol.ALLOW_EXTRA)


def tokens(text):
    """Normalize whole words while retaining Unicode letters and marks."""
    return "".join(
        char if unicodedata.category(char)[0] in "LNM" else " "
        for char in text.lower()
    ).split()


def matches_phrase(text, phrase):
    heard = " " + " ".join(tokens(text)) + " "
    expected = " ".join(tokens(phrase))
    if expected in ("ok nabu", "okay nabu"):
        return " ok nabu " in heard or " okay nabu " in heard
    return bool(expected) and f" {expected} " in heard


def wav_clip(pcm):
    output = io.BytesIO()
    with wave.open(output, "wb") as writer:
        writer.setparams((1, 2, 16000, 0, "NONE", "not compressed"))
        writer.writeframes(pcm)
    return output.getvalue()


def archive_event(directory, event, clip):
    """Save a private WAV and its metadata with the audio BLOB."""
    with closing(sqlite3.connect(directory / "wake-events.sqlite3")) as db, db:
        db.execute("""CREATE TABLE IF NOT EXISTS wake_events (
            event_id TEXT PRIMARY KEY, received_at TEXT NOT NULL,
            satellite_entity_id TEXT NOT NULL, satellite_name TEXT NOT NULL,
            wake_score REAL NOT NULL, clarity_score REAL, stt_text TEXT,
            audio_clip TEXT NOT NULL, approved INTEGER NOT NULL, reason TEXT NOT NULL,
            stt_ms INTEGER NOT NULL, clip_ms INTEGER NOT NULL, expected_phrase TEXT NOT NULL,
            stt_engine TEXT NOT NULL, audio_sha256 TEXT NOT NULL, audio_wav BLOB NOT NULL
        )""")
        db.execute("BEGIN IMMEDIATE")
        if db.execute("SELECT 1 FROM wake_events WHERE event_id = ?", (event["event_id"],)).fetchone():
            return
        path = directory / (event["event_id"] + ".wav")
        temp = path.with_suffix(".tmp")
        temp.write_bytes(clip)
        temp.replace(path)
        db.execute("""INSERT INTO wake_events VALUES (
            :event_id, :received_at, :satellite_entity_id, :satellite_name,
            :wake_score, :clarity_score, :stt_text, :audio_clip, :approved,
            :reason, :stt_ms, :clip_ms, :expected_phrase, :stt_engine,
            :audio_sha256, :audio_wav
        )""", {
            **event, "audio_clip": path.as_uri(), "audio_wav": clip,
            "audio_sha256": hashlib.sha256(clip).hexdigest(),
        })


def score(value):
    number = float(value)
    if not math.isfinite(number) or not 0 <= number <= 1:
        raise ValueError("Score outside range")
    return number


class VerifyView(HomeAssistantView):
    """Accept PCM from the configured satellite's direct LAN peer."""

    url = "/api/wake_word_verification/v1/verify"
    name = "api:wake_word_verification:verify"
    requires_auth = False

    def __init__(self, hass, settings, directory):
        self.hass = hass
        self.settings = settings
        self.directory = directory
        self.busy = False
        self.pending = set()

    async def post(self, request):
        peer = request.transport.get_extra_info("peername") if request.transport else None
        if not peer or ip_address(peer[0]) != self.settings["satellite_ip"]:
            raise web.HTTPForbidden()
        if request.content_type != "application/octet-stream":
            raise web.HTTPUnsupportedMediaType()
        try:
            event_id = str(UUID(request.headers["X-Wake-Event-Id"]))
            wake_score = score(request.headers["X-Wake-Score"])
            clarity = request.headers.get("X-Wake-Clarity")
            clarity_score = score(clarity) if clarity is not None else None
        except (KeyError, ValueError) as err:
            raise web.HTTPBadRequest(text="Invalid wake headers") from err

        if len(self.pending) >= 32:
            _LOGGER.error("Wake archive queue full for event %s", event_id)
            return self.json({"event_id": event_id, "approved": False, "reason": "archive_overload"})

        # Bound request bodies and concurrent transcription for the pilot.
        if self.busy:
            raise web.HTTPTooManyRequests()
        self.busy = True
        try:
            pcm = bytearray()
            try:
                async with asyncio.timeout(1):
                    async for chunk in request.content.iter_chunked(4096):
                        pcm.extend(chunk)
                        if len(pcm) > MAX_AUDIO_BYTES:
                            raise web.HTTPRequestEntityTooLarge(
                                max_size=MAX_AUDIO_BYTES, actual_size=len(pcm)
                            )
            except TimeoutError as err:
                raise web.HTTPRequestTimeout() from err
            if not pcm or len(pcm) % 2:
                raise web.HTTPBadRequest(text="Invalid PCM clip")

            clip = wav_clip(pcm)
            received_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
            started = time.monotonic()
            text, reason = await self.transcribe(clip)
            approved = reason == "phrase_match"
            event = {
                "event_id": event_id,
                "received_at": received_at,
                "satellite_entity_id": self.settings["satellite_entity_id"],
                "satellite_name": self.settings["satellite_name"],
                "wake_score": wake_score,
                "clarity_score": clarity_score,
                "stt_text": text,
                "approved": approved,
                "reason": reason,
                "stt_ms": round((time.monotonic() - started) * 1000),
                "clip_ms": len(pcm) // 32,
                "expected_phrase": self.settings["phrase"],
                "stt_engine": self.settings["stt_engine"],
            }
            task = self.hass.async_create_task(self.save(event, clip))
            self.pending.add(task)
            task.add_done_callback(self.pending.discard)
            return self.json({"event_id": event_id, "approved": approved, "reason": reason})
        finally:
            self.busy = False

    async def save(self, event, clip):
        try:
            await self.hass.async_add_executor_job(archive_event, self.directory, event, clip)
        except (OSError, sqlite3.Error):
            _LOGGER.error("Wake archive write failed for event %s", event["event_id"])

    async def transcribe(self, clip):
        engine = stt.async_get_speech_to_text_engine(self.hass, self.settings["stt_engine"])
        if engine is None:
            return None, "stt_unavailable"
        metadata = stt.SpeechMetadata(
            language=self.settings["language"],
            format=stt.AudioFormats.WAV,
            codec=stt.AudioCodecs.PCM,
            bit_rate=stt.AudioBitRates.BITRATE_16,
            sample_rate=stt.AudioSampleRates.SAMPLERATE_16000,
            channel=stt.AudioChannels.CHANNEL_MONO,
        )
        if not engine.check_metadata(metadata):
            return None, "stt_format_unsupported"

        async def audio():
            for offset in range(0, len(clip), 4096):
                yield clip[offset:offset + 4096]

        try:
            async with asyncio.timeout(2):
                process = (
                    engine.internal_async_process_audio_stream
                    if isinstance(engine, stt.SpeechToTextEntity)
                    else engine.async_process_audio_stream
                )
                result = await process(metadata, audio())
        except TimeoutError:
            return None, "stt_timeout"
        except Exception:
            _LOGGER.warning("Wake verification STT provider failed")
            return None, "stt_error"
        if result.result != stt.SpeechResultState.SUCCESS:
            return None, "stt_error"
        if not result.text or not result.text.strip():
            return result.text, "stt_blank"
        return result.text, (
            "phrase_match" if matches_phrase(result.text, self.settings["phrase"])
            else "phrase_mismatch"
        )


async def async_setup(hass, config):
    settings = config[DOMAIN]
    directory = Path(settings["archive_dir"])
    if not directory.is_absolute() or not await hass.async_add_executor_job(directory.is_dir):
        _LOGGER.error("Configure an existing absolute private archive directory")
        return False
    hass.http.register_view(VerifyView(hass, settings, directory))
    return True
