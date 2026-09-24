# Phase 2: История и запись в репозиторий - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-09-24
**Phase:** 2-История и запись в репозиторий
**Areas discussed:** Схема commit_files (actions[]), create_or_update_file: create vs update, Вывод diff в get_commit и compare_refs, Ветки и коммиты: фильтры и create_branch

---

## Схема commit_files (actions[])

### Типы action

| Option | Description | Selected |
|--------|-------------|----------|
| create, update, delete, move | Всё нужное агенту, включая переименование (previous_path) | ✓ |
| Только create, update, delete | Минимальная схема, переименование через delete + create | |
| Все пять (+ chmod) | Полное соответствие Commits API, chmod почти не нужен | |

**User's choice:** create, update, delete, move

### Ветка и бинарные файлы

| Option | Description | Selected |
|--------|-------------|----------|
| branch обязателен, только текст | Коммит в существующую ветку, UTF-8, без base64 | ✓ |
| branch + start_branch, текст | Новая ветка прямо в коммите | |
| branch + start_branch + encoding | Плюс encoding text/base64 на действие | |

**User's choice:** branch обязателен, только текст

### Предохранители записи

| Option | Description | Selected |
|--------|-------------|----------|
| Лимит размера + проверка массива | content ≤ 1 МБ, actions непустой и ≤ ~50, commit_message обязателен, нормализация путей | ✓ |
| Только обязательные поля | Остальное отдаём GitLab | |

**User's choice:** Лимит размера + проверка массива

### Результат записи

| Option | Description | Selected |
|--------|-------------|----------|
| SHA, ветка, строка по файлам, web_url | Агент видит, что именно записано | ✓ |
| Только SHA и web_url | Минимум, детали через get_commit | |

**User's choice:** SHA, ветка, строка по файлам, web_url

---

## create_or_update_file: create vs update

### Определение режима

| Option | Description | Selected |
|--------|-------------|----------|
| Автоопределение по GET | GET файла (404 → create, иначе update), один вызов commit_files | ✓ |
| Явное поле mode (create/update) | Модель сама выбирает режим | |
| Авто + явный отказ при конфликте | Плюс необязательное поле mode | |

**User's choice:** Автоопределение по GET

### Концы строк (CRLF)

| Option | Description | Selected |
|--------|-------------|----------|
| Пишем как есть + предупреждение | Предупреждение при CRLF→LF, ничего не меняем молча | ✓ |
| Сохранять CRLF автоматически | Сервер переприменяет доминирующий конец строки | |
| Игнорировать вопрос | Без проверок | |

**User's choice:** Пишем как есть + предупреждение

### last_commit_id

| Option | Description | Selected |
|--------|-------------|----------|
| Да, передавать автоматически | Защита от перезаписи чужого коммита, понятное сообщение при конфликте | ✓ |
| Нет | Проще код и тесты | |

**User's choice:** Да, передавать автоматически

### Ошибки записи

| Option | Description | Selected |
|--------|-------------|----------|
| Совет «ветка + MR» и подсказка про scope | Расширенный маппер ошибок | ✓ |
| Только текст GitLab в обёртке | Без собственных советов | |

**User's choice:** Совет «ветка + MR» и подсказка про scope

---

## Вывод diff в get_commit и compare_refs

### Формат в бюджете 15000

| Option | Description | Selected |
|--------|-------------|----------|
| Список файлов + патчи с лимитом на файл | Шапка, сводка, по файлу заголовок + patch до ~2000 символов | ✓ |
| Весь diff с общей обрезкой | Конкатенация и резка на 15000 | |
| Только список файлов, патч по параметру | Патч конкретного файла по path | |

**User's choice:** Список файлов + патчи с лимитом на файл

### Пустой patch и compare_timeout

| Option | Description | Selected |
|--------|-------------|----------|
| Явная причина в заголовке файла | too_large/collapsed/binary/переименование, подсказка про get_file_contents | ✓ |
| Один штамп «diff недоступен» | Без различения причин | |

**User's choice:** Явная причина в заголовке файла

### Навигация в compare_refs

| Option | Description | Selected |
|--------|-------------|----------|
| Клиентская пагинация page/per_page по файлам | Сервер режет страницы сам | ✓ |
| Фильтр path вместо пагинации | Без page | |
| Только обрезка с числом пропущенных | Нет способа дойти до хвоста | |

**User's choice:** Клиентская пагинация page/per_page по файлам

### Семантика compare_refs

| Option | Description | Selected |
|--------|-------------|----------|
| from/to, без straight | Поведение GitLab по умолчанию | ✓ |
| from/to + необязательный straight | Ещё один параметр | |
| base/head | Переименованные параметры | |

**User's choice:** from/to, без straight

---

## Ветки и коммиты: фильтры и create_branch

### Фильтры list_commits

| Option | Description | Selected |
|--------|-------------|----------|
| ref, path, since, until, author | Покрывает «что менялось в файле/за период/кем» | ✓ |
| Только ref и path | Минимум READ-06 | |
| + all и first_parent | Сложнее для модели | |

**User's choice:** ref, path, since, until, author

### list_branches

| Option | Description | Selected |
|--------|-------------|----------|
| search + строка: имя, SHA, пометки | Пометки default/protected/merged | ✓ |
| Только имя и SHA | Компактнее, без пометок | |

**User's choice:** search + строка: имя, SHA, пометки

### create_branch

| Option | Description | Selected |
|--------|-------------|----------|
| ref по умолчанию = default; «уже есть» — ошибка | Понятная ошибка, ничего не меняем молча | ✓ |
| Как выше, но «уже есть» — успех | Идемпотентно | |

**User's choice:** ref по умолчанию = default; «уже есть» — ошибка

### get_commit

| Option | Description | Selected |
|--------|-------------|----------|
| Всегда шапка + diff, без флагов | Пагинация diff, ссылка на web_url | ✓ |
| Шапка по умолчанию, diff по include_diff | Второй вызов для diff | |

**User's choice:** Всегда шапка + diff, без флагов

---

## Claude's Discretion

- Точные лимиты (patch на файл, per_page, максимум actions), формат строк и текстов ошибок, формулировки описаний инструментов.
- Набор `ToolAnnotations` для новых инструментов.
- Раскладка файлов в `internal/tools/`.

## Deferred Ideas

- `start_branch` в `commit_files`.
- Бинарная запись (`encoding: base64`) и `chmod`.
- `delete_branch` (WRT-04, v2).
- Необязательный `straight` для `compare_refs`.
