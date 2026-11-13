@tool
extends EditorPlugin

var notice: AcceptDialog
var worker: Thread

func _enter_tree() -> void:
	add_tool_menu_item("Studio: import approved credits", _credits)
	add_tool_menu_item("Studio: publish selected resource", _artifact)
	add_tool_menu_item("Studio: publish build", _publish_build)
	notice = AcceptDialog.new()
	get_editor_interface().get_base_control().add_child(notice)

func _exit_tree() -> void:
	remove_tool_menu_item("Studio: import approved credits")
	remove_tool_menu_item("Studio: publish selected resource")
	remove_tool_menu_item("Studio: publish build")
	if worker and worker.is_started():
		worker.wait_to_finish()
	notice.queue_free()

func _run(arguments: PackedStringArray) -> void:
	if worker and worker.is_started():
		return
	var python := OS.get_environment("STUDIO_PYTHON")
	if python.is_empty():
		python = "python"
	var script := OS.get_environment("STUDIO_CLIENT_PATH")
	if script.is_empty():
		notice.dialog_text = "Set STUDIO_CLIENT_PATH before starting Godot."
		notice.popup_centered()
		return
	var command := PackedStringArray([script])
	command.append_array(arguments)
	worker = Thread.new()
	worker.start(_execute.bind(python, command))

func _execute(python: String, command: PackedStringArray) -> void:
	var output: Array = []
	var code := OS.execute(python, command, output, true, false)
	_finished.call_deferred(code)

func _finished(code: int) -> void:
	worker.wait_to_finish()
	notice.dialog_text = "Studio operation completed." if code == 0 else "Studio operation pending or failed. Check the CLI outbox."
	notice.popup_centered()
	get_editor_interface().get_resource_filesystem().scan()

func _credits() -> void:
	_run(["credits", OS.get_environment("STUDIO_SNAPSHOT_ID"), ProjectSettings.globalize_path("res://credits.json")])

func _artifact() -> void:
	var paths := get_editor_interface().get_selected_paths()
	if paths.is_empty():
		return
	_run(["artifact", ProjectSettings.globalize_path(paths[0]), "--artifact-id", OS.get_environment("STUDIO_ARTIFACT_ID"), "--source", "godot"])

func _publish_build() -> void:
	_run(["build", OS.get_environment("STUDIO_BUILD_PATH"), "--source", "godot", "--release-id", OS.get_environment("STUDIO_RELEASE_ID"), "--snapshot-id", OS.get_environment("STUDIO_SNAPSHOT_ID"), "--name", OS.get_environment("STUDIO_BUILD_NAME")])
