# GitLabMcpMini

Учебный MCP-сервер (транспорт stdio) для gitlab.com: 20 компактных инструментов для чтения кода, работы с Merge Requests и записи в репозиторий. Написан для личного агента AiAdventAgentV2 (Python, `mcp` 1.30.x), который запускает MCP-серверы как stdio-подпроцессы. Результат сборки — один нативный `gitlab-mcp.exe`: Node/npx и Python на стороне сервера не нужны.

## Сборка

Нужен Go 1.25 или новее.

PowerShell:

```powershell
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp
```

bash (Git Bash, WSL):

```bash
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o gitlab-mcp.exe ./cmd/gitlab-mcp
```

Форма `CGO_ENABLED=0 go build ...` в PowerShell не запустится (синтаксическая ошибка), поэтому там переменная задаётся отдельной строкой.

Результат — один файл около 11 МБ, который импортирует только `kernel32.dll`. Это проверяет тест:

```powershell
go test ./e2e -run TestBinaryHasNoExternalDependencies
```

`gitlab-mcp.exe` перечислен в `.gitignore` и в репозиторий не попадает: собирайте его локально. Агенту передаётся абсолютный путь к этому файлу.

## Токен и scope

Сервер читает Personal Access Token из переменной окружения `GITLAB_TOKEN`.

| Что нужно | Scope токена |
|-----------|--------------|
| Только чтение (проекты, файлы, коммиты, MR, заметки) | `read_api` достаточно |
| Запись: `create_branch`, `commit_files`, `create_or_update_file`, `create_merge_request`, `create_merge_request_note`, `update_merge_request`, `merge_merge_request` | `api` обязателен |

`write_repository` не подходит: он покрывает только `git push`, а REST-эндпоинты файлов, коммитов и MR требуют `api`. Для работы только на чтение выдайте токену `read_api`.

Если `GITLAB_TOKEN` не задан, сервер сразу завершается с сообщением в stderr. Значение токена нигде не печатается: ни в логах, ни в ответах инструментов.

Необязательная переменная `GITLAB_URL` (по умолчанию `https://gitlab.com`). Поддерживается только gitlab.com.

## Подключение к AiAdventAgentV2

Агент хранит MCP-серверы как поля в своей базе данных (а не в файле `mcpServers`), поэтому сервер добавляется через форму в UI или через REST.

### Форма в UI

| Поле | Значение |
|------|----------|
| Название | `gitlab` |
| Команда | `C:\Projects\GitLabMcpMini\gitlab-mcp.exe` (путь как есть, с одинарными обратными слэшами) |
| Аргументы | пусто |
| Переменные окружения | `GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx` (по одной `KEY=VALUE` в строке; `GITLAB_URL=...` добавляйте только при необходимости) |
| Рабочая директория | пусто |
| Включён | да |

### REST

`POST /api/v1/mcp/servers` с JSON-телом (в JSON обратные слэши удваиваются, либо используйте прямые):

```json
{
  "name": "gitlab",
  "command": "C:\\Projects\\GitLabMcpMini\\gitlab-mcp.exe",
  "args": [],
  "env": {"GITLAB_TOKEN": "glpat-xxxxxxxxxxxxxxxxxxxx"},
  "cwd": null,
  "enabled": true
}
```

Токен в примерах — заглушка; подставьте свой. После сохранения значения переменных окружения в UI маскируются.

Инструменты появляются у агента под именами `mcp__gitlab__<инструмент>`, например `mcp__gitlab__list_merge_request_notes`. Ограничения самого агента: 30 секунд на вызов и 20000 символов на результат; сервер обрезает длинные ответы сам и явно помечает обрезку.

## Инструменты

Проект можно задавать числовым ID или путём `group/subgroup/project`.

| Инструмент | Что делает | Тип |
|------------|------------|-----|
| `whoami` | Текущий пользователь и scopes токена | чтение |
| `list_projects` | Проекты, доступные по токену; пометки `[scheduled for deletion]` и `[archived]` | чтение |
| `get_project` | Ключевые поля одного проекта (ветка по умолчанию, видимость, ссылка, `your_access` — роль токена, `marked_for_deletion_on` — если проект запланирован к удалению) | чтение |
| `list_repository_tree` | Дерево файлов репозитория, в том числе рекурсивно | чтение |
| `get_file_contents` | Текстовый файл целиком или по диапазону строк | чтение |
| `list_branches` | Ветки проекта | чтение |
| `create_branch` | Создаёт ветку от указанного ref | запись |
| `list_commits` | История коммитов | чтение |
| `get_commit` | Один коммит по SHA, ветке или тегу с диффом по файлам | чтение |
| `compare_refs` | Сравнение двух веток, тегов или коммитов | чтение |
| `commit_files` | Один коммит с несколькими файлами (create, update, delete, move) в существующую ветку | запись |
| `create_or_update_file` | Создаёт или обновляет один файл одним коммитом в существующую ветку | запись |
| `list_merge_requests` | Список Merge Requests | чтение |
| `get_merge_request` | Один MR по iid, включая `detailed_merge_status` | чтение |
| `get_merge_request_diffs` | Диффы MR по файлам | чтение |
| `list_merge_request_notes` | Заметки (обсуждение) MR | чтение |
| `create_merge_request` | Создаёт MR из ветки в ветку | запись |
| `create_merge_request_note` | Добавляет общий комментарий к MR | запись |
| `update_merge_request` | Меняет заголовок, описание, целевую ветку и т. п. | запись |
| `merge_merge_request` | Вливает MR после проверки статуса слияния | запись |

## Ограничения

- Набор «Mini»: другие области GitLab (issues, pipelines, wiki и т. д.) не поддерживаются.
- Запись сервером не ограничивается: это решение автора. Безопасность обеспечивается правами токена; при необходимости заведите отдельный токен и отдельный тестовый проект.
- Запросы на запись никогда не повторяются автоматически.
- Пустой файл создать нельзя: содержимое должно быть непустым, передайте один перевод строки.
- Инструмента `delete_branch` нет: исходную ветку удаляйте через `remove_source_branch` в `merge_merge_request` или в интерфейсе GitLab.
- У слияния нет защиты по `sha`: `merge_merge_request` не проверяет, что ветка не изменилась с момента чтения.
- `create_or_update_file` сам перечитывает файл непосредственно перед записью. Защиты от чужих правок, сделанных между собственным чтением агента и записью, нет.
- Проверяется только gitlab.com; self-hosted не тестировался.

## Проверка

Автотесты (без сети; `-race` на этой машине не использовать, ему нужен cgo):

```powershell
go test ./...
```

Smoke-клиент на том же `mcp==1.30.0`, что и у агента. Запуск на реальном gitlab.com выполняет только чтение (запись проверяется на локальной подделке GitLab):

```powershell
$env:GITLAB_TOKEN = "glpat-xxxxxxxxxxxxxxxxxxxx"
uv run scripts/smoke.py --project GROUP/PROJECT
```

Флаги `smoke.py`: `--exe` (путь к бинарнику, по умолчанию `gitlab-mcp.exe` в корне), `--base-url` (адрес поддельного GitLab), `--project` (обязателен без `--base-url`), `--file` (файл для чтения, по умолчанию `README.md`).

Живой прогон всего сценария на gitlab.com:

```powershell
$env:GITLAB_TOKEN = "glpat-xxxxxxxxxxxxxxxxxxxx"            # scope api
$env:GITLAB_TOKEN_READONLY = "glpat-xxxxxxxxxxxxxxxxxxxx"   # scope read_api, необязательно
uv run scripts/live.py --project GROUP/PROJECT
```

Без `GITLAB_TOKEN_READONLY` проверка read-only токена пропускается (SKIPPED). Скрипт пишет в указанный проект: создаёт ветки `live/<timestamp>-*`, вливает тестовый MR в ветку по умолчанию (файлы в `live-run/<timestamp>/` остаются в проекте), удаляет только свои ветки и MR и записывает отчёт в `.planning/phases/04-live-verification-and-delivery/LIVE-RUN.md`. Используйте отдельный тестовый проект. Дополнительные флаги: `--exe PATH`, `--out PATH`. Коды выхода: 0 (успех), 1 (провал проверки), 2 (ошибка запуска или конфигурации).
