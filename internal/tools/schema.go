package tools

// commitFilesSchema is the hand-written input schema of commit_files.
//
// The nested actions[] array is spelled out instead of being inferred from the
// Go structs: inference renders slices as a nullable type and may emit $ref
// definitions, both of which weaken how clients and models read the schema.
// Here actions is a plain {type: array, items: {type: object}} without null,
// $ref or anyOf, and both object levels reject unknown keys.
func commitFilesSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"project", "branch", "commit_message", "actions"},
		"properties": map[string]any{
			"project": map[string]any{
				"type":        "string",
				"description": "numeric project ID as a string (\"12345\") or full path group/subgroup/project",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "existing branch to commit to, for example feature/x (create it with create_branch)",
			},
			"commit_message": map[string]any{
				"type":        "string",
				"description": "commit message",
			},
			"actions": map[string]any{
				"type":        "array",
				"description": "file changes of one commit, 1..50 items",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []any{"action", "file_path"},
					"properties": map[string]any{
						"action": map[string]any{
							"type":        "string",
							"description": "create, update, delete or move",
						},
						"file_path": map[string]any{
							"type":        "string",
							"description": "path of the file in the repository",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "full new UTF-8 text content; required and non-empty for create and update (empty files are not supported, pass one newline); optional for move",
						},
						"previous_path": map[string]any{
							"type":        "string",
							"description": "old path, only for move",
						},
					},
				},
			},
		},
	}
}
