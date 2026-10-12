# podcast-transcribe

Turns a podcast episode into a transcript the project can cite. The episode is transcribed twice,
by two speech-recognition engines built on different model families, and the two passes are
aligned word by word. An agent then reconciles them with the
[`podcast-transcript` skill](../../.claude/skills/podcast-transcript/SKILL.md), reading for sense
and sentence flow, and the result is checked against both passes before it is archived.

The full text never enters the repository, which is public and built into the docs site and the
binary. It is archived outside any checkout, and the repository's documents summarise what they
need in their own words, citing the episode link and the timestamp.

## Why two engines

| Pass | Engine                                                                 | Model                                | How it decodes                                             |
| ---- | ---------------------------------------------------------------------- | ------------------------------------ | ---------------------------------------------------------- |
| A    | [openai-whisper](https://github.com/openai/whisper) (PyTorch, CPU)     | `turbo` (large-v3-turbo)             | Encoder-decoder transformer, autoregressive text           |
| B    | [parakeet-mlx](https://github.com/senstella/parakeet-mlx) (MLX, Apple) | `mlx-community/parakeet-tdt-0.6b-v2` | FastConformer encoder with a token-and-duration transducer |

Whisper writes fluent text and sometimes smooths over what was said: it drops stutters, normalises
"gonna" to "going to", and can repeat or invent a phrase in a silence. Parakeet keeps closer to the
sounds and makes different mistakes ("live for your diachron" for "live free or die"). Where the
two agree, the words are very probably what was heard; where they differ, the worksheet shows both
readings with each engine's confidence. Agreement is not proof: both engines heard the guest's
name as "Steinbeck", which is why the worksheet compares capitalised words against the episode
notes.

Neither engine identifies speakers. Paragraphs and their attribution come from reading the
conversation, and the published transcript says so.

## Setup

```bash
make install-transcribe   # pipx installs of openai-whisper and parakeet-mlx, unless already on PATH
```

Needs `ffmpeg`, `pipx` and, for pass B, Apple Silicon. The first run downloads the models: about
1.6 GB into `~/.cache/whisper` and 2.5 GB into `~/.cache/huggingface`. On other hardware, run
pass B as a second Whisper model (`--pass-b openai-whisper:medium.en`); the tool warns that two
passes from one engine share their errors.

## Use

```bash
T=tools/podcast-transcribe/podcast-transcribe.py
python3 $T resolve  'https://podcasts.apple.com/us/podcast/…/id1610029625?i=1000793374806'
python3 $T prepare  'https://castro.fm/episode/esLjgi'
python3 $T check    ~/.cache/velocity-report/podcast-transcribe/<episode>
python3 $T publish  ~/.cache/velocity-report/podcast-transcribe/<episode> --out-dir <archive>
```

- `resolve` prints the episode a link points to. Apple Podcasts links resolve through the iTunes
  lookup API, Castro pages carry the audio URL, and Spotify pages give the show and episode titles,
  which are found in the show's public RSS feed. A direct audio URL also works. Spotify-exclusive
  episodes have no public audio and are refused.
- `prepare` downloads the audio, converts it to 16 kHz mono, runs both passes, aligns them and
  writes `worksheet.md`. Finished passes are reused unless `--force` is given. On an M1 Pro, a
  26-minute episode took 18 minutes for pass A and 37 seconds for pass B.
- `check` reads the agent's `reconciled.md` and fails on a run of four or more words that neither
  pass heard (an invention) or four or more words that both passes heard and the transcript left
  out (an omission). Shorter runs, such as spelling corrections, are listed for review.
- `publish` runs the check, then writes `transcript.md` with its provenance table, beside the
  evidence for it, to `<archive>/<show>/<released>-<title>/`. The archive is `--out-dir` or
  `$PODCAST_TRANSCRIPT_DIR`; the owner's is `/Volumes/lidar/lidar/transcripts`. It refuses any
  directory inside a Git checkout, and prints the transcript's SHA-256 for citing.

## Work directory

Working files stay outside the checkout, in
`~/.cache/velocity-report/podcast-transcribe/<released>-<title>/` (override with `--work-root`):

| File                       | Contents                                                                                    |
| -------------------------- | ------------------------------------------------------------------------------------------- |
| `source.mp3`, `audio.wav`  | The downloaded enclosure and its 16 kHz mono conversion                                     |
| `passes/<engine>-<model>/` | Each engine's native JSON, its log and `run.json` (command, version, wall time, audio hash) |
| `state.json`               | The resolved episode, both runs and the agreement statistics                                |
| `worksheet.md`             | The aligned passes in chunks, for the agent to reconcile                                    |
| `reconciled.md`            | The agent's transcript: speakers header, then timestamped paragraphs                        |
| `check.json`               | The last check's result                                                                     |

These are raw outputs under the repository's rule for experiments: never commit them. `publish`
copies all of them except `audio.wav`, which is derived from the source, into the archive beside
the transcript. The transcript records the audio's SHA-256, so the passes can be reproduced from
the same enclosure.

## Tests

```bash
python3 -m unittest tools/podcast-transcribe/test_podcast_transcribe.py
```

The tests use canned pages, feeds and engine output; they need no network and no engine.
