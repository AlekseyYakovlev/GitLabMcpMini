# LIVE-RUN: живой прогон gitlab-mcp на gitlab.com

- Дата (UTC): 2026-09-25T09:47:34Z
- Проект: AlekseyYakovlev/sanbox
- Метка прогона: 20260925-094734
- exe: C:\Projects\GitLabMcpMini\gitlab-mcp.exe
- Размер: 11446784 байт
- SHA-256: 955f26d02ebd808d3126d41ad7b159d29113695d5eb82171a681a4d9f6853a71
- Актуальность: exe не старее исходников
- mcp client: 1.30.0
- protocolVersion: 2025-11-25

go version -m (выдержка):

````text
dep	github.com/modelcontextprotocol/go-sdk	v1.8.0	h1:KIvahhYqwtbeniWVPs3TcXEA7b8jEtwfBpOTAI+Urx4=
dep	gitlab.com/gitlab-org/api/client-go/v2	v2.64.0	h1:PrLDC9rS6je6scB0VgXAh6gmoB7fk50bgdweKT37Vdg=
build	-trimpath=true
build	CGO_ENABLED=0
````

## Критерии

| Критерий | Статус |
|----------|--------|
| 1. Основной сценарий: чтение, ветка, коммит, MR, комментарий, merge, очистка | PASS |
| 2. Реальные ошибки: дубли, 404, пустой content, Draft, 401, read-only 403 | PASS |
| Сборка без внешних зависимостей (CGO_ENABLED=0), размер 11446784 байт | PASS |
| Очистка | PASS |

## Шаги

### 1. 0 tools/list — PASS

- tool: `list_tools`
- примечание: инструментов: 20, ожидалось 20

````json
{}
````

````text
commit_files, compare_refs, create_branch, create_merge_request, create_merge_request_note, create_or_update_file, get_commit, get_file_contents, get_merge_request, get_merge_request_diffs, get_project, list_branches, list_commits, list_merge_request_notes, list_merge_requests, list_projects, list_repository_tree, merge_merge_request, update_merge_request, whoami
````

### 2. 1 whoami — PASS

- tool: `whoami`
- примечание: scopes содержит api (либо scopes unavailable)

````json
{}
````

````text
[identity redacted] PASS: пользователь токена получен
token: scopes [api], expires 2027-09-24
````

### 3. 2 get_project — PASS

- tool: `get_project`
- примечание: есть строка default_branch

````json
{
  "project": "AlekseyYakovlev/sanbox"
}
````

````text
id: 86873071
path: AlekseyYakovlev/sanbox
default_branch: main
visibility: private
archived: no
last_activity: 2026-09-24
web_url: https://gitlab.com/AlekseyYakovlev/sanbox
````

### 4. 3 list_repository_tree — PASS

- tool: `list_repository_tree`
- примечание: в корне есть README.md

````json
{
  "project": "AlekseyYakovlev/sanbox"
}
````

````text
AlekseyYakovlev/sanbox @ main (ветка по умолчанию), путь: /
file README.md
[page 1, per_page 20 — последняя страница]
````

### 5. 4 get_file_contents README.md — PASS

- tool: `get_file_contents`

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "README.md",
  "ref": "main"
}
````

````text
README.md @ main — строки 1-93 из 93, 6017 байт
# Sanbox



## Getting started

To make it easy for you to get started with GitLab, here's a list of recommended next steps.

Already a pro? Just edit this README.md and make it your own. Want to make it easy? [Use the template at the bottom](#editing-this-readme)!

## Add your files

* [Create](https://docs.gitlab.com/user/project/repository/web_editor/#create-a-file) or [upload](https://docs.gitlab.com/user/project/repository/web_editor/#upload-a-file) files
* [Add files using the command line](https://docs.gitlab.com/topics/git/add_files/#add-files-to-a-git-repository) or push an existing Git repository with the following command:

```
cd existing_repo
git remote add origin https://gitlab.com/AlekseyYakovlev/sanbox.git
git branch -M main
git push -uf origin main
```

## Integrate with your tools

* [Set up project integrations](https://gitlab.com/AlekseyYakovlev/sanbox/-/settings/integrations)

## Collaborate with your team

* [Invite team members and collaborators](https://docs.gitlab.com/user/project/members/)
* [Create a new merge request](https://docs.gitlab.com/user/project/merge_requests/creating_merge_requests/)
* [Automatically close issues from merge requests](https://docs.gitlab.com/user/project/issues/managing_issues/#closing-issues-automatically)
* [Enable merge request approvals](https://docs.gitlab.com/user/project/merge_requests/approvals/)
* [Set auto-merge](https://docs.gitlab.com/user/project/merge_requests/auto_merge/)

## Test and Deploy

Use the built-in continuous integration in GitLab.

* [Get started with GitLab CI/CD](https://docs.gitlab.com/ci/quick_start/)
* [Analyze your code for known vulnerabilities with Static Application Security Testing (SAST)](https://docs.gitlab.com/user/application_security/sast/)
* [Deploy to Kubernetes, Amazon EC2, or Amazon ECS using Auto Deploy](https://docs.gitlab.com/topics/autodevops/requirements/)
* [Use pull-based deployments for improved Kubernetes management](https://docs.gitlab.com/user/clusters/agent/)
* [Set up protected environments](https://docs.gitlab.com/ci/environments/protected_environments/)

***

# Editing this README

When you're ready to make this README your own, just edit this file and use the handy template below (or feel free to structure it however you want - this is just a starting point!). Thanks to [makeareadme.com](https://www.makeareadme.com/) for this template.

## Suggestions for a good README

Every project is different, so consider which of these sections apply to yours. The sections used in the template are suggestions for most open source projects. Also keep in mind that while a README can be too long and detailed, too long is better than too short. If you think your README is too long, consider utilizing another form of documentation rather than cutting out information.

## Name
Choose a self-explaining name for your project.

## Description
Let people know what your project can do specifically. Provide context and add a link to any reference visitors might be unfamiliar with. A list of Features or a Background subsection can also be added here. If there are alternatives to your project, this is a good place to list differentiating factors.

## Badges
On some READMEs, you may see small images that convey metadata, such as whether or not all the tests are passing for the project. You can use Shields to add some to your README. Many services also have instructions for adding a badge.

## Visuals
Depending on what you are making, it can be a good idea to include screenshots or even a video (you'll frequently see GIFs rather than actual videos). Tools like ttygif can help, but check out Asciinema for a more sophisticated method.

## Installation
Within a particular ecosystem, there may be a common way of installing things, such as using Yarn, NuGet, or Homebrew. However, consider the possibility that whoever is reading your README is a novice and would like more guidance. Listing specific steps helps remove ambiguity and gets people to using your project as quickly as possible. If it only runs in a specific context like a particular programming language version or operating system or has dependencies that have to be installed manually, also add a Requirements subsection.

## Usage
Use examples liberally, and show the expected output if you can. It's helpful to have inline the smallest example of usage that you can demonstrate, while providing links to more sophisticated examples if they are too long to reasonably include in the README.

## Support
Tell people where they can go to for help. It can be any combination of an issue tracker, a chat room, an email address, etc.

## Roadmap
If you have ideas for releases in the future, it is a good idea to list them in the README.

## Contributing
State if you are open to contributions and what your requirements are for accepting them.

For people who want to make changes to your project, it's helpful to have some documentation on how to get started. Perhaps there is a script that they should run or some environment variables that they need to set. Make these steps explicit. These instructions could also be useful to your future self.

You can also document commands to lint the code or run tests. These steps help to ensure high code quality and reduce the likelihood that the changes inadvertently break something. Having instructions for running tests is especially helpful if it requires external setup, such as starting a Selenium server for testing in a browser.

## Authors and acknowledgment
Show your appreciation to those who have contributed to the project.

## License
For open source projects, say how it is licensed.

## Project status
If you have run out of energy or time for your project, put a note at the top of the README saying that development has slowed down or stopped completely. Someone may choose to fork your project or volunteer to step in as a maintainer or owner, allowing your project to keep going. You can also make an explicit request for maintainers.
````

### 6. 5 create_branch — PASS

- tool: `create_branch`
- примечание: ответ называет новую ветку

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-main",
  "ref": "main"
}
````

````text
ветка live/20260925-094734-main создана от main
конец ветки: 4b2e1fe36f7ce3b6fc71fdf3d14c9cfb0a634c22
https://gitlab.com/AlekseyYakovlev/sanbox/-/tree/live/20260925-094734-main
````

### 7. 6 commit_files — PASS

- tool: `commit_files`
- примечание: коммит с тремя строками create

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-main",
  "commit_message": "live 20260925-094734: add files",
  "actions": [
    {
      "action": "create",
      "file_path": "live-run/20260925-094734/a.md",
      "content": "# live-run/20260925-094734/a.md\nlive run 20260925-094734\n"
    },
    {
      "action": "create",
      "file_path": "live-run/20260925-094734/b.md",
      "content": "# live-run/20260925-094734/b.md\nlive run 20260925-094734\n"
    },
    {
      "action": "create",
      "file_path": "live-run/20260925-094734/notes.md",
      "content": "# live-run/20260925-094734/notes.md\nlive run 20260925-094734\n"
    }
  ]
}
````

````text
коммит 91f86d90 в live/20260925-094734-main (+6/−0): 3 файла
create live-run/20260925-094734/a.md (+2/−0)
create live-run/20260925-094734/b.md (+2/−0)
create live-run/20260925-094734/notes.md (+2/−0)
https://gitlab.com/AlekseyYakovlev/sanbox/-/commit/91f86d90fce87c65fdf239598bd852ced7f8c2d0
````

### 8. 7a create_or_update_file (update) — PASS

- tool: `create_or_update_file`
- примечание: ответ начинается с updated

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "live-run/20260925-094734/notes.md",
  "content": "# notes\nlive run 20260925-094734\nupdated\n",
  "branch": "live/20260925-094734-main",
  "commit_message": "live 20260925-094734: update notes"
}
````

````text
updated live-run/20260925-094734/notes.md: коммит 4bc1152b в live/20260925-094734-main
https://gitlab.com/AlekseyYakovlev/sanbox/-/commit/4bc1152b0360f09d3f2ea07d18b10885781a90cd
````

### 9. 7b create_or_update_file (create) — PASS

- tool: `create_or_update_file`
- примечание: ответ начинается с created

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "live-run/20260925-094734/c.md",
  "content": "# c\nlive run 20260925-094734\n",
  "branch": "live/20260925-094734-main",
  "commit_message": "live 20260925-094734: add c"
}
````

````text
created live-run/20260925-094734/c.md: коммит 096c1139 в live/20260925-094734-main
https://gitlab.com/AlekseyYakovlev/sanbox/-/commit/096c1139f8fe8955fd55a0ee85f00317e3209b1d
````

### 10. 8 get_commit — PASS

- tool: `get_commit`
- примечание: в коммите виден a.md

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "sha": "91f86d90"
}
````

````text
коммит 91f86d90fce87c65fdf239598bd852ced7f8c2d0
автор: api_token, дата: 2026-09-25T09:47:37Z
родители: 4b2e1fe3
изменения: +6/−0
сообщение:
live 20260925-094734: add files
https://gitlab.com/AlekseyYakovlev/sanbox/-/commit/91f86d90fce87c65fdf239598bd852ced7f8c2d0
файлов на странице: 3
### live-run/20260925-094734/a.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/a.md
+live run 20260925-094734

### live-run/20260925-094734/b.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/b.md
+live run 20260925-094734

### live-run/20260925-094734/notes.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/notes.md
+live run 20260925-094734

[page 1, per_page 20 — последняя страница]
````

### 11. 9 compare_refs — PASS

- tool: `compare_refs`
- примечание: в сравнении виден c.md

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "from": "main",
  "to": "live/20260925-094734-main"
}
````

````text
сравнение AlekseyYakovlev/sanbox: main → live/20260925-094734-main
коммитов: 3
91f86d90 2026-09-25 api_token live 20260925-094734: add files
4bc1152b 2026-09-25 api_token live 20260925-094734: update notes
096c1139 2026-09-25 api_token live 20260925-094734: add c
файлов: 4, на странице: 1-4
### live-run/20260925-094734/a.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/a.md
+live run 20260925-094734

### live-run/20260925-094734/b.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/b.md
+live run 20260925-094734

### live-run/20260925-094734/c.md [new]
@@ -0,0 +1,2 @@
+# c
+live run 20260925-094734

### live-run/20260925-094734/notes.md [new]
@@ -0,0 +1,3 @@
+# notes
+live run 20260925-094734
+updated

[page 1, per_page 20 — последняя страница]
````

### 12. 10 create_merge_request — PASS

- tool: `create_merge_request`
- примечание: «MR !N создан»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "source_branch": "live/20260925-094734-main",
  "target_branch": "main",
  "title": "live 20260925-094734: gitlab-mcp live run",
  "description": "Создан scripts/live.py, прогон 20260925-094734."
}
````

````text
MR !1 создан: live/20260925-094734-main→main
draft: нет
detailed_merge_status: preparing — GitLab ещё готовит diff MR; повторите get_merge_request через несколько секунд
статус слияния обычно ещё checking: перед merge_merge_request вызовите get_merge_request
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/1
````

### 13. 11 create_merge_request_note — PASS

- tool: `create_merge_request_note`
- примечание: «комментарий #ID добавлен к MR !1»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1,
  "body": "live 20260925-094734: note"
}
````

````text
комментарий #3903154203 добавлен к MR !1
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/1
````

### 14. 12 list_merge_request_notes — PASS

- tool: `list_merge_request_notes`
- примечание: комментарий виден в списке

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1
}
````

````text
заметки MR !1 проекта AlekseyYakovlev/sanbox (новые первыми)
#3903154203 @project_86873071_bot_704d90f4aef6ecad8b896cab62bf198f 2026-09-25
live 20260925-094734: note
[page 1, per_page 20 — последняя страница]
````

### 15. 13 get_merge_request_diffs — PASS

- tool: `get_merge_request_diffs`
- примечание: в diff виден a.md

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1
}
````

````text
!1 live 20260925-094734: gitlab-mcp live run
ветки: live/20260925-094734-main→main
файлов на странице: 4
### live-run/20260925-094734/a.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/a.md
+live run 20260925-094734

### live-run/20260925-094734/b.md [new]
@@ -0,0 +1,2 @@
+# live-run/20260925-094734/b.md
+live run 20260925-094734

### live-run/20260925-094734/c.md [new]
@@ -0,0 +1,2 @@
+# c
+live run 20260925-094734

### live-run/20260925-094734/notes.md [new]
@@ -0,0 +1,3 @@
+# notes
+live run 20260925-094734
+updated

[page 1, per_page 20 — последняя страница]
````

### 16. ожидание статуса mergeable для MR !1 — PASS

- tool: `get_merge_request`
- примечание: detailed_merge_status=mergeable, ожидалось mergeable, опросов: 1

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1
}
````

````text
!1 live 20260925-094734: gitlab-mcp live run
state: opened
автор: @project_86873071_bot_704d90f4aef6ecad8b896cab62bf198f
ветки: live/20260925-094734-main→main
detailed_merge_status: mergeable — можно вливать (merge_merge_request)
признак конфликтов: нет
pipeline: нет
файлов: 4
описание:
Создан scripts/live.py, прогон 20260925-094734.
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/1
````

### 17. 15 merge_merge_request — PASS

- tool: `merge_merge_request`
- примечание: «MR !1 влит» и SHA коммита

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1,
  "remove_source_branch": true
}
````

````text
MR !1 влит (state=merged)
merge commit: 7f58ddc870ead6030e71bd794d4472c1256bfbf6
удаление ветки-источника: запрошено
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/1
````

### 18. 16 get_merge_request после merge — PASS

- tool: `get_merge_request`
- примечание: попыток: 1

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1
}
````

````text
!1 live 20260925-094734: gitlab-mcp live run
state: merged
автор: @project_86873071_bot_704d90f4aef6ecad8b896cab62bf198f
ветки: live/20260925-094734-main→main
MR уже влит (state=merged)
detailed_merge_status: not_open — MR не открыт; смотрите state (влит или закрыт); закрытый можно открыть: update_merge_request state_event=reopen
признак конфликтов: нет
pipeline: нет
файлов: 4
описание:
Создан scripts/live.py, прогон 20260925-094734.
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/1
````

### 19. 17 get_file_contents (после merge) — PASS

- tool: `get_file_contents`
- примечание: c.md в ветке по умолчанию содержит ts

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "live-run/20260925-094734/c.md",
  "ref": "main"
}
````

````text
live-run/20260925-094734/c.md @ main — строки 1-2 из 2, 29 байт
# c
live run 20260925-094734
````

### 20. 18 list_branches: ветка-источник удалена — PASS

- tool: `list_branches`
- примечание: WARN: ветка ещё не удалена (удаление асинхронное), cleanup удалит

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "search": "live/20260925-094734-main"
}
````

````text
ветки AlekseyYakovlev/sanbox, поиск: live/20260925-094734-main
ветки не найдены
````

### 21. E1a create_branch (для Draft MR) — PASS

- tool: `create_branch`
- примечание: ветка создана

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-draft",
  "ref": "main"
}
````

````text
ветка live/20260925-094734-draft создана от main
конец ветки: 7f58ddc870ead6030e71bd794d4472c1256bfbf6
https://gitlab.com/AlekseyYakovlev/sanbox/-/tree/live/20260925-094734-draft
````

### 22. E1b create_branch (дубль) — PASS

- tool: `create_branch`
- примечание: isError, «ветка уже существует»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-draft",
  "ref": "main"
}
````

````text
400: ветка уже существует: выберите другое имя или используйте существующую ветку.
````

### 23. E2 commit_files (diff для Draft MR) — PASS

- tool: `commit_files`
- примечание: коммит создан

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-draft",
  "commit_message": "live 20260925-094734: draft",
  "actions": [
    {
      "action": "create",
      "file_path": "live-run/20260925-094734/draft.md",
      "content": "draft 20260925-094734\n"
    }
  ]
}
````

````text
коммит 4e609fe9 в live/20260925-094734-draft (+1/−0): 1 файл
create live-run/20260925-094734/draft.md (+1/−0)
https://gitlab.com/AlekseyYakovlev/sanbox/-/commit/4e609fe92374aa95c4487ebc719803468f26d850
````

### 24. E3 commit_files (create без content) — PASS

- tool: `commit_files`
- примечание: isError, «непустой content»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-draft",
  "commit_message": "live 20260925-094734: empty",
  "actions": [
    {
      "action": "create",
      "file_path": "live-run/20260925-094734/empty.md"
    }
  ]
}
````

````text
actions[0]: для create нужен непустой content (пустой файл создать нельзя: передайте один перевод строки)
````

### 25. E4a create_merge_request (draft) — PASS

- tool: `create_merge_request`
- примечание: «MR !N создан»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "source_branch": "live/20260925-094734-draft",
  "target_branch": "main",
  "title": "live 20260925-094734: draft",
  "draft": true
}
````

````text
MR !2 создан: live/20260925-094734-draft→main
draft: да
detailed_merge_status: preparing — GitLab ещё готовит diff MR; повторите get_merge_request через несколько секунд
статус слияния обычно ещё checking: перед merge_merge_request вызовите get_merge_request
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/2
````

### 26. ожидание статуса draft_status для MR !2 — PASS

- tool: `get_merge_request`
- примечание: detailed_merge_status=draft_status, ожидалось draft_status, опросов: 2

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 2
}
````

````text
!2 Draft: live 20260925-094734: draft
state: opened [draft]
автор: @project_86873071_bot_704d90f4aef6ecad8b896cab62bf198f
ветки: live/20260925-094734-draft→main
detailed_merge_status: draft_status — MR помечен как Draft; снимите пометку: update_merge_request с title без префикса «Draft:»
признак конфликтов: нет
pipeline: нет
файлов: 1
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/2
````

### 27. E4b merge_merge_request (Draft) — PASS

- tool: `merge_merge_request`
- примечание: isError, причина draft_status/Draft

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 2
}
````

````text
слияние не выполнено: detailed_merge_status: draft_status — MR помечен как Draft; снимите пометку: update_merge_request с title без префикса «Draft:»
````

### 28. E5 create_merge_request (дубль) — PASS

- tool: `create_merge_request`
- примечание: isError, «открытый MR уже есть: !2»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "source_branch": "live/20260925-094734-draft",
  "target_branch": "main",
  "title": "live 20260925-094734: draft dup"
}
````

````text
открытый MR уже есть: !2. Используйте его (get_merge_request) или закройте.
````

### 29. E6 update_merge_request (close) — PASS

- tool: `update_merge_request`
- примечание: MR закрыт

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 2,
  "state_event": "close"
}
````

````text
MR !2 обновлён: state_event
state: closed
draft: да
ветки: live/20260925-094734-draft→main
https://gitlab.com/AlekseyYakovlev/sanbox/-/merge_requests/2
````

### 30. E7 get_file_contents (нет файла) — PASS

- tool: `get_file_contents`
- примечание: isError, «404: не найдено»

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "live-run/20260925-094734/no-such-file.md",
  "ref": "main"
}
````

````text
404: не найдено (файл). GitLab также отвечает 404, если токен не видит приватный проект.
````

### 31. E8 get_file_contents (нет ветки) — PASS

- tool: `get_file_contents`
- примечание: isError, 404

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "path": "README.md",
  "ref": "live/20260925-094734-no-such-branch"
}
````

````text
404: не найдено (файл). GitLab также отвечает 404, если токен не видит приватный проект.
````

### 32. E9 get_project (нет проекта) — PASS

- tool: `get_project`
- примечание: isError, «404: не найдено»

````json
{
  "project": "AlekseyYakovlev/gitlab-mcp-live-missing-20260925-094734"
}
````

````text
404: не найдено (проект). GitLab также отвечает 404, если токен не видит приватный проект.
````

### 33. E10 merge_merge_request (уже влит) — PASS

- tool: `merge_merge_request`
- примечание: isError, MR уже влит / not_open

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "iid": 1
}
````

````text
MR уже влит (state=merged)
````

### 34. B whoami (невалидный токен) — PASS

- tool: `whoami`
- примечание: isError, «401: GitLab отклонил токен», токен не в тексте

````json
{}
````

````text
401: GitLab отклонил токен (недействителен, просрочен или отозван). Проверьте GITLAB_TOKEN.
````

### 35. C1 whoami (read-only токен) — PASS

- tool: `whoami`
- примечание: токен read-only без scope api

````json
{}
````

````text
[identity redacted] PASS: пользователь токена получен
token: scopes [read_api], expires 2027-09-24
````

### 36. C2 list_branches (read-only токен) — PASS

- tool: `list_branches`
- примечание: чтение read-only токеном работает

````json
{
  "project": "AlekseyYakovlev/sanbox"
}
````

````text
ветки AlekseyYakovlev/sanbox
live/20260925-094734-draft 4e609fe9 live 20260925-094734: draft
main 7f58ddc8 [default] [protected] Merge branch 'live/20260925-094734-main' into 'main'
[page 1, per_page 20 — последняя страница]
````

### 37. C3 create_branch (read-only токен) — PASS

- tool: `create_branch`
- примечание: isError, «403: запись отклонена», подсказка про scope `api`

````json
{
  "project": "AlekseyYakovlev/sanbox",
  "branch": "live/20260925-094734-ro",
  "ref": "main"
}
````

````text
403: запись отклонена: ветка может быть защищена, у токена может не быть scope `api`, или ваша роль в проекте ниже Developer. Создайте свою ветку (create_branch), коммитьте в неё, затем откройте MR.
````

## cleanup (вне сервера)

- ветка live/20260925-094734-main: уже отсутствует (404)
- ветка live/20260925-094734-draft: удалена
- ветка live/20260925-094734-ro: уже отсутствует (404)
