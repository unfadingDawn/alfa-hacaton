# Third-party components

This prototype uses the components below. Exact dependency versions, including transitive dependencies, are fixed in
`backend/go.sum` and `frontend/pnpm-lock.yaml`. No third-party model weights are bundled with the repository.

## Backend libraries

| Component | Purpose | License |
|---|---|---|
| chi | HTTP router | MIT |
| golang-jwt/jwt | JWT handling | MIT |
| google/uuid | UUID handling | BSD-3-Clause |
| pgx | PostgreSQL driver | MIT |
| goose | Database migrations | MIT |
| Gonum | Linear algebra for local TF-IDF + LSA ranking | BSD-3-Clause |
| golang.org/x/crypto | Argon2 password hashing | BSD-3-Clause |
| golang.org/x/text | Text processing support | BSD-3-Clause |

## Frontend libraries and assets

Next.js, React, Tailwind CSS, Radix UI, Motion, Recharts, Lucide, Sonner, class-variance-authority, clsx and
tailwind-merge are used under their respective open-source licenses recorded by their packages. Inter font files are
provided by Fontsource under the SIL Open Font License.

## Runtime images

- `pgvector/pgvector:pg16`: PostgreSQL with pgvector, PostgreSQL License.
- `golang:1.24-alpine`, `node:22-alpine`, `alpine:3.20`: official language/runtime and Alpine Linux images; their
  included packages retain their respective licenses.

## Models and services

The application can call an organizer-provided OpenAI-compatible gateway. It only accepts model identifiers allowed by
the case and validated in `backend/internal/llm/allowed.go`: Gemma 4 26B A4B, GLM-5.3-Flash and Qwen3.6 35B A3B are the
configured defaults. Model weights are not distributed by this repository. The local TF-IDF + LSA ranker is trained
from ingested news at runtime and does not use external model weights or an external AI service.

## Data

- Anonymized company, relationship and payment CSV files are supplied by the hackathon organizers and are not committed.
- Alfa-Bank research materials are supplied as case materials; OCR-derived text is stored under `data/research/`.
- External news metadata is fetched from the RSS sources configured in the application. Source URLs and attribution are
  retained with each article.
