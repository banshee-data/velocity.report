---
name: podcast-transcript
description: Transcribe a podcast episode (Apple Podcasts, Castro, Spotify or a direct audio link) with two independent speech-recognition passes, reconcile them by reading for sense and sentence flow, verify the result against both passes, archive the full text outside the repository, and summarise what the docs need with timestamped citations.
argument-hint: "<episode URL>"
---

# Skill: podcast-transcript

Produce a transcript the project can cite. The tool does the listening twice, with engines whose
mistakes differ; this skill does what neither engine can: decide what was said where they disagree,
notice where they agree on nonsense, punctuate, paragraph, and attribute each paragraph to a
speaker. A check then holds the result to the evidence. The tool and its files are described in
[tools/podcast-transcribe/README.md](../../../tools/podcast-transcribe/README.md).

**The full text stays out of the repository.** The repository is public, and `docs/` and `data/`
are built into the docs site and the binary, so committing a transcript would republish someone
else's episode. The transcript is archived outside any checkout (the owner's archive is
`/Volumes/lidar/lidar/transcripts`), and documents carry only a summary in the project's own words,
short quotations, and a citation of the episode link with timestamps.

## Usage

```
/podcast-transcript https://podcasts.apple.com/us/podcast/…/id1610029625?i=1000793374806
/podcast-transcript https://castro.fm/episode/esLjgi
/podcast-transcript https://open.spotify.com/episode/6CchCq5oCwRIGasE881NOf
```

## Procedure

### 1. Preflight

- `df -h ~`: the first run downloads about 4 GB of models; each episode needs its audio twice
  over (about 100 MB for half an hour). Stop and say so if the disk is short.
- `make install-transcribe`, which reuses engines already on PATH.
- `python3 tools/podcast-transcribe/podcast-transcribe.py resolve <URL>` and confirm the show,
  title and date are the episode the owner meant.

### 2. Transcribe

Run `prepare <URL>` in the background. Pass A (Whisper on CPU) takes about 0.7 times the episode's
length on an M1 Pro; pass B (Parakeet) takes seconds. It prints the work directory and the
agreement between passes. Agreement below about 95% means something went wrong (music beds, a
second language, a Whisper repetition loop): look at the worksheet before reconciling.

### 3. Establish the speakers

Read the worksheet's header and episode notes, then the opening and closing minutes, where hosts
introduce themselves and their guests. Name a speaker only from the episode itself, its notes or
feed, or the publisher's own pages; otherwise write `Host` or `Guest`. A name taken from outside
the episode goes in the `notes:` line with where it came from. Use the publisher's spelling of
names and of the show (the worksheet's `⟨notes spell: …⟩` hints point at likely mishearings).

### 4. Reconcile, chunk by chunk

Read the worksheet in order, one chunk at a time, with the next chunk in view so sentences that
cross a boundary come out whole. Write `reconciled.md` in the work directory as you go:

```
speakers:
- Kea Wilson: host, Streetsblog USA
- Rebecca Steinbach: guest, AAA Foundation for Traffic Safety
notes: One or two sentences for the published introduction: sources of names, conventions.
---
[00:00:08] Kea Wilson: Hey guys, it's Kea. Welcome back to The Brake. …

[00:01:00] It turns out there isn't just one answer to that question. …

[00:02:36] Rebecca Steinbach: Sure. So actually, believe it or not, …
```

A paragraph starts with the `[hh:mm:ss]` of its first word, taken from the nearest worksheet
marker. A declared speaker's name and a colon open each turn; a paragraph without one continues
the same speaker. Start a paragraph at every change of speaker and, inside long turns, every 100
to 200 words at a sentence boundary.

Rules for the words:

1. **Divergences.** For each `⟦A: … | B: …⟧`, choose the reading that makes grammatical and
   semantic sense in the sentence and fits what both speakers say elsewhere. When both make sense,
   prefer the higher confidence; confidences from different engines are not calibrated against
   each other, so treat a small difference as a tie and prefer B's literal reading. When one pass
   heard nothing (`∅`), keep the words if they fit the flow (Whisper drops short phrases) and drop
   them if they repeat what was just said or stand in a silence (a Whisper loop).
2. **Agreement is not proof.** Read the agreed text for sense too. Where both passes heard a word
   that cannot be right and a near-homophone plainly is ("follow the reflux", where the host later
   says "reflexes"), write the near-homophone followed by `[?]`. Correct a proper noun to the
   publisher's spelling without a mark. Otherwise leave agreed words alone, even when the speaker
   misspoke; add a bracketed note if a misstatement would mislead a reader, such as a group name
   that differs from the report's.
3. **Never compose.** Do not paraphrase, summarise, tidy grammar, finish a sentence the speaker
   abandoned, or reorder. An interrupted sentence ends with `…`.
4. **Clean verbatim.** Leave out "um", "uh" and their kin, stutters ("it was it was") and immediate
   self-corrections that repeat a word ("these this group"). Keep "you know", "like", "I mean",
   false starts that change course, and everything said in ads, credits and calls to action.
5. **Spelling.** Write the speakers' own variety of English: a transcript is a quotation, and
   the repository's British spelling applies to its own prose. Write reduced forms out when a pass
   did ("going to" over "gonna"); keep forms both passes heard ("gotta"). Numbers as the passes
   wrote them; "a thousand" over "1000" when one pass spelled it out.
6. **Editorial marks** go in square brackets and are excluded from the check: `[?]` after a doubted
   word, `[inaudible]` where neither pass is usable, `[crosstalk]`, `[laughs]`, and short notes.

### 5. Check

Run `check <work dir>`. Fix every failure: a run of four or more words neither pass heard is
composition; four or more words both passes heard and the transcript lacks is an omission. Then
read every shorter item it lists and confirm each is a deliberate spelling or a removed stutter.

### 6. Archive

Run `publish <work dir> --out-dir <archive>` (or set `PODCAST_TRANSCRIPT_DIR`) and read the output
in full. It writes `transcript.md` with its provenance beside the source audio, both passes, the
worksheet, `reconciled.md` and `check.json`, and refuses a directory inside a Git checkout. Note
the SHA-256 it prints. Commit any tool change first, so the provenance names a commit that contains
the tool; republish if the tool changed.

### 7. Summarise where the docs need it

Only what a document uses goes into the repository, in the document that uses it:

- A summary in the project's own words, with the timestamp of each point, and quotations of a
  phrase or a sentence, not passages. Say what the speaker is (an author describing their own
  study, a host) and keep spoken characterisations apart from the source's published figures: an
  interview is a secondary account of a report, and a figure heard in it is not read from the
  report.
- A row in the document's sources table and a reference link to the episode's public page, naming
  the archived transcript by its SHA-256 and the archive's relative path (`transcripts/<show>/…`).
- A `@misc` entry in `data/maths/references.bib` when a plan cites the episode as a source.
- The house rules still apply: the wording guard, British spelling in the project's prose, and the
  line between what people say and what the sensor measures.

Commit with explicit paths: `[ai][docs] summarise <show>, "<episode>", in <document>`. Never stage
the work directory or the archive.

## What this skill does not do

It does not listen. Every judgement rests on two machine passes and the text around them; the
published status says the transcript has not been checked against the audio by a person. It does
not identify speakers by voice. It does not translate.
