package github

import "testing"

func TestParseAndSetChecklist(t *testing.T) {
	body := "## Tasks\n\n- [ ] one\n- [x] two\n* [ ] three\n"
	tasks := parseChecklistTasks(body)
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks", len(tasks))
	}
	if tasks[0].Done || !tasks[1].Done || tasks[2].Done {
		t.Fatalf("done flags: %+v", tasks)
	}
	if tasks[0].Body != "one" || tasks[2].ID != "checklist-2" {
		t.Fatalf("parse: %+v", tasks)
	}

	next, err := setChecklistDone(body, "checklist-0", true)
	if err != nil {
		t.Fatal(err)
	}
	tasks = parseChecklistTasks(next)
	if !tasks[0].Done || !tasks[1].Done || tasks[2].Done {
		t.Fatalf("after set: %+v", tasks)
	}

	next, err = setChecklistDone(next, "checklist-1", false)
	if err != nil {
		t.Fatal(err)
	}
	tasks = parseChecklistTasks(next)
	if !tasks[0].Done || tasks[1].Done {
		t.Fatalf("after unset: %+v", tasks)
	}
}
