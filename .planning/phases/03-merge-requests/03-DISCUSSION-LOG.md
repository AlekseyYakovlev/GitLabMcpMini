# Phase 3: Merge Requests - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-09-24
**Phase:** 03-merge-requests
**Areas discussed:** Список и детали MR, Diff и комментарии MR, Создание и правка MR, Merge и его отказы

---

## Список и детали MR

| Option | Description | Selected |
|--------|-------------|----------|
| Только opened, все авторы | state=opened, scope=all; фильтры state/source/target/author/search | ✓ |
| Все состояния (state=all) | Без фильтра по состоянию, список быстро разрастается | |
| opened + фильтр «мои» (scope) | Добавляет scope created_by_me/assigned_to_me | |

**User's choice:** Только opened, все авторы

| Option | Description | Selected |
|--------|-------------|----------|
| Короткая шапка + пояснение статуса | Ключевые поля, detailed_merge_status = сырое значение + русское пояснение | ✓ |
| Только сырой статус | Без перевода значений | |
| Расширенная карточка | + reviewers, assignees, labels, milestone, approvals | |

**User's choice:** Короткая шапка + пояснение статуса

| Option | Description | Selected |
|--------|-------------|----------|
| Сообщить честно, не ждать | Один HTTP-запрос, «повторите через несколько секунд» | ✓ |
| Короткий полинг в merge | Только merge опрашивает 2–3 раза | |
| Полинг и в get, и в merge | Ждёт до ~5 с в обоих | |

**User's choice:** Сообщить честно, не ждать (checking/unchecked)

---

## Diff и комментарии MR

| Option | Description | Selected |
|--------|-------------|----------|
| Все в хронологии, системные помечены [system] | Цельная история, соответствует MR-04 | ✓ |
| По умолчанию скрыты, флаг include_system | Меньше шума, теряется событийная картина | |

**User's choice:** Все в хронологии, системные помечены [system]

| Option | Description | Selected |
|--------|-------------|----------|
| Старые первыми (asc), тело до ~1000 | Читается как дискуссия | |
| Новые первыми (desc, как у GitLab), тело до ~1000 | Свежие на первой странице | ✓ |
| Старые первыми, без обрезки тел до общего бюджета | Один огромный комментарий вытесняет остальные | |

**User's choice:** Новые первыми (desc, как у GitLab), тело до ~1000

| Option | Description | Selected |
|--------|-------------|----------|
| Тот же рендер + явный overflow | renderDiffFiles без переделок, пагинация GitLab, /diffs | ✓ |
| Больший бюджет на файл | ~4000 на файл, расхождение с get_commit | |
| Параметр path для одного файла | Фильтрация на сервере, расширяет объём фазы | |

**User's choice:** Тот же рендер + явный overflow

---

## Создание и правка MR

| Option | Description | Selected |
|--------|-------------|----------|
| Минимум параметров, безопасные умолчания | source_branch, title, target по умолчанию ветка проекта, description, draft | ✓ |
| + squash и remove_source_branch в create | Дублирует параметры merge | |
| + assignee/reviewer/labels | За рамками «Mini» | |

**User's choice:** Минимум параметров, безопасные умолчания

| Option | Description | Selected |
|--------|-------------|----------|
| Понятная ошибка + ссылка на существующий | isError «открытый MR уже есть: !N»; пустой diff — «нет изменений» | ✓ |
| Идемпотентно: вернуть существующий как успех | Тихо игнорирует расхождение title/description | |

**User's choice:** Понятная ошибка + ссылка на существующий

| Option | Description | Selected |
|--------|-------------|----------|
| Непустое = меняем, очистка не поддержана | title, description, target_branch, state_event, draft; документируется | ✓ |
| Специальный флаг clear_description | Лишнее поле для редкого случая | |
| Только title/description/state | Без draft и target_branch | |

**User's choice:** Непустое = меняем, очистка не поддержана

---

## Merge и его отказы

| Option | Description | Selected |
|--------|-------------|----------|
| Предпроверка и отказ с пояснением, без auto-merge | GET MR → не mergeable → isError без PUT | ✓ |
| Предпроверка + опция auto_merge | Состояние «запланировано» не отслеживается | |
| Без предпроверки, только маппинг ошибок PUT | Не соответствует критерию «сервер проверяет статус» | |

**User's choice:** Предпроверка и отказ с пояснением, без auto-merge

| Option | Description | Selected |
|--------|-------------|----------|
| squash, remove_source_branch, merge_commit_message; умолчания проекта | Не переданные не отправляются | ✓ |
| Только remove_source_branch и squash | Без merge_commit_message | |
| Никаких опций | Ветка остаётся после merge | |

**User's choice:** squash, remove_source_branch, merge_commit_message; умолчания проекта

| Option | Description | Selected |
|--------|-------------|----------|
| Взять 999.2 в Phase 3 и показывать состояние MR | Тексты «результат неизвестен» + подсказка get_merge_request | ✓ |
| Только новые MR-тексты, 999.2 остаётся в бэклоге | Риск дубля merge/комментария | |

**User's choice:** Взять 999.2 в Phase 3 и показывать состояние MR

---

## Claude's Discretion

- Точные лимиты (описание ~1500, тело заметки ~1000, per_page ~20), формат строк и русские тексты таблицы статусов.
- Источник числа файлов и `overflow` для diff; способ поиска существующего MR при 409.
- Раскладка файлов в `internal/tools/`; механика `draft` в `update_merge_request`.

## Deferred Ideas

- Inline-комментарии (MRX-01), approve (MRX-02), sha-защита merge (MRX-03), `delete_branch` (WRT-04) — v2.
- Auto-merge при идущем pipeline; очистка description/флагов; assignee/reviewer/labels; `scope` списка; `path`-фильтр diff.
