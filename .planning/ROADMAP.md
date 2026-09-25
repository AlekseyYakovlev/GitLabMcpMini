# Roadmap: GitLabMcpMini

## Milestones

- ✅ **v1.0 MVP** — Phases 1-4 (shipped 2026-09-25)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-4) — SHIPPED 2026-09-25</summary>

- [x] Phase 1: Подключение и чтение проекта (6/6 plans) — completed 2026-09-24
- [x] Phase 2: История и запись в репозиторий (5/5 plans) — completed 2026-09-24
- [x] Phase 3: Merge Requests (5/5 plans) — completed 2026-09-24
- [x] Phase 4: Живая проверка и поставка (4/4 plans) — completed 2026-09-25

Full details: [milestones/v1.0-ROADMAP.md](./milestones/v1.0-ROADMAP.md)

</details>

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Подключение и чтение проекта | v1.0 | 6/6 | Complete | 2026-09-24 |
| 2. История и запись в репозиторий | v1.0 | 5/5 | Complete | 2026-09-24 |
| 3. Merge Requests | v1.0 | 5/5 | Complete | 2026-09-24 |
| 4. Живая проверка и поставка | v1.0 | 4/4 | Complete | 2026-09-25 |

## Backlog

### Phase 999.1: commit_files: create/update без content не должен коммитить пустой файл (CLOSED in Phase 4, plan 04-01)

**Goal:** Сделать `ActionIn.Content` указателем (`*string`), отклонять create/update без `content` до запроса, разрешить явную пустую строку; тест «нет запроса без content». Источник: 02-REVIEW CR-01, 02-VERIFICATION. Рекомендуется закрыть до живого прогона Phase 4 (тихая деструктивная запись).
**Closed:** реализовано без указателя (D-10): `content` остаётся `string`, пустой `content` при create/update в `commit_files` и в `create_or_update_file` отклоняется до запроса; пустой файл не создаётся (обход: один перевод строки); тесты «нет запроса без content».
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.2: Тексты ошибок записи: «запись могла примениться» для Canceled/Decode/TooLarge/Other (CLOSED in Phase 3, plan 03-03)

**Goal:** Добавить эти виды сбоев в ветку `writeUnknownOutcome` в `errors.go` (`writeText`), чтобы модель не повторяла запись и не создавала дубль коммита. Источник: 02-REVIEW WR-01.
**Closed:** writeText covers Canceled/Decode/TooLarge/Other with op-specific hints (Phase 3, D-15).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.3: create_or_update_file: описание завышает защиту от чужих правок (BACKLOG)

**Goal:** Либо добавить входной `last_commit_id`, либо исправить описание инструмента: сервер сам перечитывает файл перед POST. Источник: 02-REVIEW WR-02 (и WR-05: перезапись бинарного файла текстом как обычное «updated»).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.4: compare_refs: переполнение при огромном page (BACKLOG)

**Goal:** Ограничить `page` (и `page+1` в `PageFooter`), чтобы вместо recovered-паники в `safe` возвращалось понятное сообщение «страница вне диапазона». Источник: 02-REVIEW WR-03.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)

### Phase 999.5: smoke.py: записи только против loopback/фейка, не против gitlab.com (BACKLOG)

**Goal:** Запускать `create_branch`, `commit_files` и перезапись README только при loopback-хосте или явном флаге. Иначе любой `--base-url`, включая gitlab.com, считается «hermetic». Обязательно до Phase 4. Источник: 02-REVIEW WR-04.
**Status:** write guard implemented in Phase 3 (plan 03-03, `is_loopback` in scripts/smoke.py): all smoke writes run only against 127.0.0.1/localhost/::1. Close after the author confirms.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /bm:review-backlog when ready)
