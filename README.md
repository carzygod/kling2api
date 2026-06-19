# KLING-CREATOR-01

`KLING-CREATOR-01` is a Kling Web-session reverse-proxy service for `https://klingai.com/app`.

It does not use Kling official API keys and does not require Redis. Account material, account events, and media tasks are stored in SQLite.

## Scope

Implemented:

- Admin WebUI at `/admin?key=<KLING_CREATOR_ADMIN_KEY>`.
- Headless Chromium login session capture for Kling Web cookies.
- Manual cookie import.
- SQLite account pool.
- Account list, delete, and transport test.
- OpenAI-style model list at `/v1/models`.
- Image generation at `/v1/images/generations`.
- Video generation at `/v1/videos/generations`.
- Synchronous video generation at `/v1/videos/generations/sync`.
- Task polling at `/v1/tasks/{id}` and `/v1/videos/generations/{id}`.
- Text-to-image, image-to-image, text-to-video, image-to-video.
- First/last-frame video model wrapper.
- Action-clone model wrapper.
- Raw Kling Web payload override through `payload` or `metadata.raw_payload`.

Not implemented:

- Guaranteed real-time quota sync. Kling Web quota fields still need upstream page/API capture.

## Environment

```env
HOST=0.0.0.0
PORT=18013
DATA_DIR=/data
DATABASE_PATH=/data/kling-creator-01.sqlite
KLING_CREATOR_ADMIN_KEY=replace-with-admin-key
KLING_CREATOR_AUTH_KEY=replace-with-api-key
KLING_LOGIN_URL=https://klingai.com/app
CHROME_EXECUTABLE=/usr/bin/chromium
```

## Docker

```bash
docker build -t kling-creator-01:local .
docker run -d --name kling-creator-01 \
  -p 18013:18013 \
  -e KLING_CREATOR_ADMIN_KEY=replace-with-admin-key \
  -e KLING_CREATOR_AUTH_KEY=replace-with-api-key \
  -v kling-creator-01-data:/data \
  kling-creator-01:local
```

## Login Flow

1. Open `/admin?key=<KLING_CREATOR_ADMIN_KEY>`.
2. Create a login session.
3. Complete Kling login in the screenshot flow.
4. Capture the logged-in session.
5. The service stores cookies, cookie string, localStorage JSON, and user-agent in SQLite.

Admin APIs require `?key=<KLING_CREATOR_ADMIN_KEY>`, `X-Admin-Key`, or `Authorization: Bearer <KLING_CREATOR_ADMIN_KEY>`.

Media APIs require `Authorization: Bearer <KLING_CREATOR_AUTH_KEY>` or `X-API-Key`.

## Models

`GET /v1/models` returns the exact model ids exposed by this wrapper. Extra fields such as `endpoint`, `provider_version`, `web_task_types`, and `support_status` are included so NewAPI or a portal can distinguish verified models from mapped aliases.

Image models:

| Model | Endpoint | Upstream payload | Version | Status |
|---|---|---|---|---|
| `kling-image` | `/v1/images/generations` | `mmu_txt2img_aiweb` | 1.0 | mapped |
| `kling-image-v1-5` | `/v1/images/generations` | `mmu_txt2img_aiweb` / `mmu_img2img_aiweb` | 1.5 | mapped |
| `kling-image-v2` | `/v1/images/generations` | `mmu_txt2img_aiweb` / `mmu_img2img_aiweb` | 2.0 | mapped |
| `kling-image-v2-1` | `/v1/images/generations` | `mmu_txt2img_aiweb` / `mmu_img2img_aiweb` | 2.1 | verified for text-to-image on SH01 |
| `kling-image-i2i` | `/v1/images/generations` | `mmu_img2img_aiweb` | 2.1 default | mapped alias |

Video models:

| Model | Endpoint | Upstream payload | Version | Status |
|---|---|---|---|---|
| `kling-v1` / `kling-video-1.0` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 1.0 | mapped |
| `kling-v1-5` / `kling-video-1.5` | `/v1/videos/generations` | `m2v_img2video` | 1.5 | mapped |
| `kling-v1-6` / `kling-video-1.6` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 1.6 | mapped |
| `kling-v2-master` / `kling-video-2.0` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 2.0 | mapped |
| `kling-v2-1` / `kling-v2-1-master` / `kling-video-2.1` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 2.1 | mapped |
| `kling-video-2.1-hq` | `/v1/videos/generations` | `m2v_txt2video_hq` / `m2v_img2video_hq` | 2.1 | mapped |
| `kling-v2-5-turbo` / `kling-video-2.5-turbo` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 2.5 | mapped |
| `kling-v2-6` | `/v1/videos/generations` | `m2v_txt2video` / `m2v_img2video` | 2.6 | mapped |
| `kling-v3` / `kling-video-3.0` | `/v1/videos/generations` | `m2v_omni_video` | 3.0-omni | verified for text-to-video on SH01 |
| `kling-video-first-last-frame` | `/v1/videos/generations` | `m2v_img2video` with `tail_image` | 2.1+ | mapped |
| `kling-action-clone` | `/v1/videos/generations` | `m2v_motion_clone` | 2.1+ | mapped |
| `kling-video-extend` | `/v1/videos/generations` | `m2v_extend_video` | 1.5+ | mapped |

Known unsupported / not mapped:

- `kling-image-v3` is intentionally not listed. Testing on SH01 showed that simply sending `kolors_version: 3.0` to the legacy `mmu_txt2img_aiweb` task returns upstream `TASK.InvalidTaskType`, and a guessed `kling_img_web` task returns `BASE.OperationUnsupported`.
- In the current wrapper, "Kling 3.0" means video generation via `kling-v3` or `kling-video-3.0`. It uses Kling Web's current Omni payload: `type=m2v_omni_video`, `kling_version=3.0-omni`, and `model_mode=720p` by default.

## Image Generation

Text to image:

```bash
curl http://127.0.0.1:18013/v1/images/generations \
  -H "Authorization: Bearer $KLING_CREATOR_AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kling-image",
    "prompt": "a cute cat, cinematic lighting",
    "aspect_ratio": "1:1",
    "n": 1
  }'
```

Image to image:

```json
{
  "model": "kling-image-i2i",
  "prompt": "turn this into anime style",
  "image_url": "https://example.com/input.png",
  "aspect_ratio": "1:1",
  "n": 1
}
```

Supported image input forms:

- `https://...` URL.
- `data:image/png;base64,...` data URI. The service uploads it to Kling before task submission.

## Video Generation

Video task responses are aligned with `QIANWEN-CREATOR-01`:

- `object`: `video.generation.task`
- `status`: `queued`, `running`, `completed`, `failed`, or `cancelled`
- task id fields: `id` and `task_id`
- polling field: `poll_url`
- local cancellation at `POST /v1/video/generations/{task_id}/cancel` and `POST /v1/videos/generations/{task_id}/cancel`
- result URL fields: `data[0].url`, `data[0].video_url`, `url`, and `video_url`

Asynchronous text to video:

```bash
curl http://127.0.0.1:18013/v1/videos/generations \
  -H "Authorization: Bearer $KLING_CREATOR_AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "kling-video-2.1",
    "prompt": "a white cube slowly rotating on a desk, realistic photography",
    "duration": 5,
    "aspect_ratio": "16:9",
    "mode": "std"
  }'
```

Poll:

```bash
curl http://127.0.0.1:18013/v1/tasks/task_xxx \
  -H "Authorization: Bearer $KLING_CREATOR_AUTH_KEY"
```

Synchronous video:

```bash
curl http://127.0.0.1:18013/v1/videos/generations/sync \
  -H "Authorization: Bearer $KLING_CREATOR_AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{
  "model": "kling-video-2.1",
  "prompt": "a white cube slowly rotating on a desk",
  "duration": 5,
  "aspect_ratio": "16:9",
  "generate_audio": false
}'
```

Image to video:

```json
{
  "model": "kling-video-2.1",
  "prompt": "the subject smiles and waves",
  "image_url": "https://example.com/start.png",
  "duration": 5
}
```

First/last-frame video:

```json
{
  "model": "kling-video-first-last-frame",
  "prompt": "smooth transition between the two frames",
  "first_frame_image": "https://example.com/first.png",
  "last_frame_image": "https://example.com/last.png",
  "duration": 5
}
```

Action clone:

```json
{
  "model": "kling-action-clone",
  "prompt": "make the character follow the reference motion",
  "first_frame_image": "https://example.com/character.png",
  "action_video_url": "https://example.com/motion.mp4",
  "duration": 5
}
```

## Raw Payload Override

If Kling changes the Web payload shape, callers can submit an exact captured payload without waiting for a code change:

```json
{
  "model": "kling-video-2.1",
  "prompt": "kept for task metadata",
  "payload": {
    "type": "m2v_txt2video",
    "inputs": [],
    "arguments": [
      {"name": "prompt", "value": "a white cube slowly rotating"},
      {"name": "duration", "value": "5"},
      {"name": "kling_version", "value": "2.1"},
      {"name": "aspect_ratio", "value": "16:9"},
      {"name": "biz", "value": "klingai"}
    ]
  }
}
```

## API Summary

```text
GET  /health
GET  /admin?key=...
GET  /api/summary
GET  /api/accounts
POST /api/accounts
DELETE /api/accounts/{id}
POST /api/accounts/{id}/test
GET  /api/login-sessions
POST /api/login-sessions
GET  /api/login-sessions/{id}/screenshot
POST /api/login-sessions/{id}/refresh
POST /api/login-sessions/{id}/click
POST /api/login-sessions/{id}/input
POST /api/login-sessions/{id}/capture
DELETE /api/login-sessions/{id}
GET  /v1/models
POST /v1/images/generations
POST /v1/videos/generations
POST /v1/videos/generations/sync
GET  /v1/videos/generations/{task_id}
GET  /v1/tasks/{task_id}
```

## SQLite Tables

```text
kling_accounts
kling_account_events
kling_tasks
```

The account pool stores only upstream Kling Web login material. It does not store NewAPI channel keys or official API keys.
