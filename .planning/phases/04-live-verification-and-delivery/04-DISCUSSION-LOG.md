# Phase 4: Живая проверка и поставка - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-09-24
**Phase:** 4-Живая проверка и поставка
**Areas discussed:** Формат живого прогона, Тестовый проект/токены/очистка, Бэклог 999.x до живого прогона, Поставка: README и сборка

---

## Формат живого прогона

### Как выполнять сквозной сценарий на gitlab.com

| Option | Description | Selected |
|--------|-------------|----------|
| Отдельный live-скрипт (Recommended) | Новый scripts/live.py на mcp==1.30.0, явный opt-in, транскрипт; smoke.py остаётся герметичным | ✓ |
| Расширить smoke.py флагом --live-write | Один скрипт на оба режима; разрастается и делит код с герметичными тестами | |
| Вручную через агент/Inspector | Ближе к реальному использованию, не воспроизводимо | |

**User's choice:** Отдельный live-скрипт

### Фиксация результата

| Option | Description | Selected |
|--------|-------------|----------|
| Транскрипт в .planning/phases/04-*/ (Recommended) | LIVE-RUN.md с шагами, ответами, PASS/FAIL; токен редактируется | ✓ |
| Только вывод в консоль + строка в VERIFICATION | Легче, без полного следа | |

**User's choice:** Транскрипт в .planning/phases/04-*/

### Баги, найденные живым прогоном

| Option | Description | Selected |
|--------|-------------|----------|
| Чинить в Phase 4 (Recommended) | Фиксы входят в фазу, прогон повторяется; для каждого — герметичный тест | ✓ |
| Только блокирующее, остальное в бэклог | Блокеры сразу, мелочи в 999.x | |

**User's choice:** Чинить в Phase 4

---

## Тестовый проект, токены, очистка

### Проект для живой записи

| Option | Description | Selected |
|--------|-------------|----------|
| Постоянный пустой sandbox (Recommended) | Приватный проект с README в main, уникальные имена веток | ✓ |
| Реальный проект автора | Мусорит историю | |
| Скрипт создаёт и удаляет проект сам | Чисто, но нужен REST мимо сервера и права владельца | |

**User's choice:** Постоянный пустой sandbox

### Передача токенов

| Option | Description | Selected |
|--------|-------------|----------|
| Две env-переменные (Recommended) | GITLAB_TOKEN (api) обязателен; GITLAB_TOKEN_READONLY опционален, без него SKIPPED | ✓ |
| Две обязательные переменные | Без обоих скрипт не стартует | |

**User's choice:** Две env-переменные

### Очистка без delete_branch

| Option | Description | Selected |
|--------|-------------|----------|
| Скрипт сам через REST (Recommended) | В finally закрывает свои MR и удаляет ветки live/<ts>-* | ✓ |
| Только remove_source_branch | Остальное вручную | |
| Добавить delete_branch в сервер | 21-й инструмент, выход за границы фазы | |

**User's choice:** Скрипт сам через REST

### Реальные ошибки сверх критерия (multiSelect)

| Option | Description | Selected |
|--------|-------------|----------|
| Невалидный токен → 401 | | ✓ |
| Несуществующая ветка/файл → 404 | | ✓ |
| Отказ merge (draft / неполная проверка) | | ✓ |
| Дубль: вторая ветка/MR с тем же именем | | ✓ |

**User's choice:** Все четыре

---

## Бэклог 999.x до живого прогона

### Что закрываем (multiSelect)

| Option | Description | Selected |
|--------|-------------|----------|
| 999.1 content у commit_files (Recommended) | Тихая запись пустого файла — нельзя выпускать без фикса | ✓ |
| 999.3 описание create_or_update_file | | |
| 999.4 page overflow в compare_refs | | |
| 999.5 — подтвердить закрытие | | |

**User's choice:** Только 999.1

### Реализация 999.1

| Option | Description | Selected |
|--------|-------------|----------|
| Пустой content отклоняется (Recommended) | string с omitempty, ошибка до запроса, пустой файл создать нельзя | ✓ |
| *string + ручная правка схемы | Позволяет пустые файлы, усложняет схему и golden | |

**User's choice:** Пустой content отклоняется

---

## Поставка: README и сборка

### Язык и состав README

| Option | Description | Selected |
|--------|-------------|----------|
| Русский, компактный (Recommended) | Сборка, токен/scope, поля command/args/env агента, 20 инструментов, ограничения, проверка | ✓ |
| Английский | | |
| Два языка (RU + EN) | | |

**User's choice:** Русский, компактный

### Сборка и поставка exe

| Option | Description | Selected |
|--------|-------------|----------|
| Команда в README + скрипт-проверка (Recommended) | Одна каноническая команда, exe в .gitignore, проверка отсутствия зависимостей и размер в транскрипте | ✓ |
| Плюс build-скрипт | | |
| Прикладывать exe в репо/релиз | | |

**User's choice:** Команда в README + скрипт-проверка

---

## Claude's Discretion

- Структура live.py, имена веток/MR, паузы для статуса `checking`
- Формулировки описания `commit_files` и разделов README
- Форма проверки сборки, вычисление SHA/размера exe
- Раскладка задач и волн, необходимость gap-closure планов

## Deferred Ideas

- `delete_branch` (WRT-04), запись пустого файла, 999.3/999.4/999.5, v2-функции (SRCH-01, MRX-*, UX-*), build-скрипт, публикация exe, RU+EN README, проверка внутри самого агента
