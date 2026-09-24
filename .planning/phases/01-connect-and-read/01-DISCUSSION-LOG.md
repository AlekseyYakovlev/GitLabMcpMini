# Phase 1: Подключение и чтение проекта - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-09-24
**Phase:** 1-Подключение и чтение проекта
**Areas discussed:** Клиент GitLab, Имена инструментов, Формат вывода, Чтение файла

---

## Клиент GitLab

| Option | Description | Selected |
|--------|-------------|----------|
| Свой net/http (Recommended) | ~300 строк, полный контроль кодирования путей, GET-only ретраев, редактирования токена | |
| client-go v2 | Типизированные опции, ретраи из коробки; тяжёлый граф зависимостей, мажорные релизы раз в ~6 месяцев | ✓ |
| Time-boxed спайк | Подключить два инструмента на обоих вариантах, потом решить | |

**User's choice:** client-go v2 (вопреки рекомендации)
**Notes:** Ретраи и редактирование токена в ошибках client-go нужно проверить и настроить.

## Ретраи GET

| Option | Description | Selected |
|--------|-------------|----------|
| До 2 повторов, ≤10 с (Recommended) | Учитывает Retry-After, укладывается в 25 с | ✓ |
| Без ретраев вообще | Проще, но 5xx чаще ломают сценарии | |
| Решит Claude | Запись не повторяется, бюджет ≤25 с | |

**User's choice:** До 2 повторов, ≤10 с

## Имена инструментов

| Option | Description | Selected |
|--------|-------------|----------|
| Без префикса (Recommended) | list_projects, get_file_contents, whoami | ✓ |
| Префикс gitlab_ | gitlab_list_projects; защита от коллизий, длиннее имена | |

**User's choice:** Без префикса

## Формат вывода

| Option | Description | Selected |
|--------|-------------|----------|
| Компактный текст (Recommended) | Одна строка на элемент + футер | ✓ |
| Урезанный JSON | Массив объектов и объект пагинации | |
| Смесь | Текст для списков, JSON для одиночных объектов | |

**User's choice:** Компактный текст

## Футер обрезки и пагинации

| Option | Description | Selected |
|--------|-------------|----------|
| Одна строка-подсказка (Recommended) | Человекочитаемая подсказка для модели | ✓ |
| Структура в конце | Машинная строка next_page=2 truncated=false | |

**User's choice:** Одна строка-подсказка

## Лимит размера get_file_contents

| Option | Description | Selected |
|--------|-------------|----------|
| Общий бюджет ~15 000 символов (Recommended) | Голова файла + пометка про start_line/end_line | ✓ |
| Отдельный лимит на файл | Например до 50 КБ | |

**User's choice:** Общий бюджет ~15 000 символов

## Бинарные файлы

| Option | Description | Selected |
|--------|-------------|----------|
| Пометка без содержимого (Recommended) | Определение по NUL/невалидному UTF-8, без base64 | ✓ |
| Отдавать base64 по запросу | Флаг encoding=base64 | |

**User's choice:** Пометка без содержимого

## Ref по умолчанию

| Option | Description | Selected |
|--------|-------------|----------|
| Ветка по умолчанию проекта (Recommended) | default_branch из GET /projects/:id, указывается в выводе | ✓ |
| HEAD | Передаём ref=HEAD напрямую | |

**User's choice:** Ветка по умолчанию проекта

---

## Claude's Discretion

- Раскладка пакетов и имя Go-модуля
- `per_page` по умолчанию и лимит рекурсивного дерева
- Точный формат текстовых строк и набор полей
- Способ ограничения ретраев client-go и таймаутов
- Расположение smoke-скрипта

## Deferred Ideas

- Проверка размера итогового `.exe` с client-go v2 (заметка для Phase 4)
