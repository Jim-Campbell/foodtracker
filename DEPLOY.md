# Deploying Food to Render

Click-through instructions — nothing here is scripted, since cloud resources
shouldn't be created by an agent. Follow in order.

## 1. Database

1. Render dashboard → **New → PostgreSQL**.
2. Name it `food-db` (or similar), pick the region closest to you, free or
   starter plan is fine to begin with.
3. Once created, copy the **Internal Database URL** — you'll paste this into
   the web service's `DATABASE_URL` in step 3. (Internal, not External: the
   web service and DB will live in the same Render region/network.)

## 2. Generate the API key

```sh
openssl rand -hex 24
```

Save this value — it's `FOOD_API_KEY` below, and the same value you paste
into the PWA's first-run prompt on your phone.

## 3. Web service

1. Render dashboard → **New → Web Service** → connect this repo
   (`jimgcampbell/food` or wherever it's hosted).
2. Environment: **Docker** (Render auto-detects the `Dockerfile`).
3. Region: same as the database from step 1.
4. Plan: starter is fine to begin with.
5. Environment variables:

   | Key | Value |
   |---|---|
   | `DATABASE_URL` | the Internal Database URL from step 1 |
   | `FOOD_API_KEY` | the value generated in step 2 |
   | `ANTHROPIC_API_KEY` | your Anthropic API key |
   | `FDC_API_KEY` | free key from https://api.data.gov/signup/ |
   | `R2_ACCOUNT_ID` | reuse journal's R2 credentials |
   | `R2_ACCESS_KEY_ID` | reuse journal's R2 credentials |
   | `R2_SECRET_ACCESS_KEY` | reuse journal's R2 credentials |
   | `R2_BUCKET` | a new bucket, or the `food/` prefix in journal's existing bucket |
   | `R2_PUBLIC_URL` | matching public URL for the bucket/prefix above |

   `AI_MODEL` and `PORT` are optional — leave unset for defaults
   (`claude-sonnet-5`, `8082`; Render sets its own `$PORT` and the app reads
   it, so `PORT` usually doesn't need setting on Render at all — confirm the
   service's assigned port matches what's exposed).

6. Deploy. **Migrations run automatically at startup** — first boot creates
   the whole schema from `internal/db/migrations/`, nothing to run by hand.
7. Once live, hit `https://<your-service>.onrender.com/api/health` and
   confirm `{"ok":true,"photos":true,"ai":true}` (both `true` once the R2 and
   Anthropic vars above are set correctly).

## 4. Install on iPhone

1. Open the Render URL in **Safari** (must be Safari, not Chrome, for PWA
   install on iOS).
2. Share button → **Add to Home Screen**.
3. Open the new Home Screen icon — first run prompts for the API key; paste
   the value from step 2.
4. Log a test meal to confirm everything's wired up end to end.

## Notes

- Render's disk is ephemeral — there's no persistent volume in this setup.
  The backup story is `GET /api/export` (Settings → Export in the PWA):
  download it periodically, or set up your own recurring job that hits the
  endpoint and stores the result somewhere durable.
- If R2 or Anthropic vars are missing/wrong, the app still runs — camera
  logging and/or AI parsing are just disabled (`/api/health` reports which).
  Nothing crashes on missing optional config.
