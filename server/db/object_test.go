package db

import (
	"reflect"
	"testing"
)

func Test_Object_Owns(t *testing.T) {
	o1 := Object{
		OwnerID: 1,
	}
	o11 := Object{
		OwnerID: 1,
	}
	o2 := Object{
		OwnerID: 2,
	}
	cs := []struct {
		name     string
		rcv      Object
		tgt      Object
		expected bool
	}{
		{
			name:     "owns",
			rcv:      o1,
			tgt:      o11,
			expected: true,
		},
		{
			name: "does not own",
			rcv:  o1,
			tgt:  o2,
		},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			r := &c.rcv
			result := r.HasSameOwner(c.tgt)
			if result != c.expected {
				t.Errorf("expected %v, got %v", c.expected, result)
			}
		})
	}
}

func Test_Object_Can(t *testing.T) {
	o1 := Object{
		OwnerID: 1,
	}
	o11 := Object{
		OwnerID: 1,
	}
	o2 := Object{
		OwnerID: 2,
		Perms: &Permissions{
			Read:  PermWorld,
			Write: PermOwner,
			Exec:  PermOwner,
			Carry: PermOwner,
		},
	}
	o3 := Object{
		OwnerID: 3,
		Perms: &Permissions{
			Read:  PermOwner,
			Write: PermWorld,
			Exec:  PermOwner,
			Carry: PermOwner,
		},
	}
	o4 := Object{
		OwnerID: 4,
		Perms: &Permissions{
			Read:  PermOwner,
			Write: PermOwner,
			Exec:  PermWorld,
			Carry: PermOwner,
		},
	}
	o5 := Object{
		OwnerID: 5,
		Perms: &Permissions{
			Read:  PermOwner,
			Write: PermOwner,
			Exec:  PermOwner,
			Carry: PermWorld,
		},
	}
	cs := []struct {
		name     string
		rcv      Object
		perm     string
		tgt      Object
		expected bool
	}{
		{
			name:     "o is self",
			rcv:      o1,
			perm:     "write",
			tgt:      o1,
			expected: true,
		},
		{
			name:     "owner matches",
			rcv:      o1,
			perm:     "write",
			tgt:      o11,
			expected: true,
		},
		{
			name:     "not owner but read ok",
			rcv:      o1,
			perm:     "read",
			tgt:      o2,
			expected: true,
		},
		{
			name:     "not owner but write ok",
			rcv:      o1,
			perm:     "write",
			tgt:      o3,
			expected: true,
		},
		{
			name:     "not owner but exec ok",
			rcv:      o1,
			perm:     "exec",
			tgt:      o4,
			expected: true,
		},
		{
			name:     "not owner but carry ok",
			rcv:      o1,
			perm:     "carry",
			tgt:      o5,
			expected: true,
		},
		{
			name: "not owner, read not ok",
			rcv:  o1,
			perm: "read",
			tgt:  o5,
		},
		{
			name: "not owner, write not ok",
			rcv:  o1,
			perm: "write",
			tgt:  o4,
		},
		{
			name: "not owner, exec not ok",
			rcv:  o1,
			perm: "exec",
			tgt:  o3,
		},
		{
			name: "not owner, carry not ok",
			rcv:  o1,
			perm: "carry",
			tgt:  o2,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			r := &c.rcv
			result := r.Can(c.perm, c.tgt)
			if result != c.expected {
				t.Errorf("expected %v, got %v", c.expected, result)
			}
		})
	}
}

func Test_Object_SetScript(t *testing.T) {
	cs := []struct {
		name           string
		input          string
		expectedData   map[string]string
		expectedPerms  Permissions
		expectedScript string
		err            string
	}{
		{
			name: "basic",
			input: `--[[WITCH
data:
  foo: bar
  baz: quux
permissions:
  read: world
  write: owner
  exec: world
  carry: owner
--HCTIW]]

hears(".*dance.*", function()
	does("bounces lightly")
end)
`,
			expectedData: map[string]string{
				"foo": "bar",
				"baz": "quux",
			},
			expectedPerms: Permissions{
				Read:  PermWorld,
				Write: PermOwner,
				Exec:  PermWorld,
				Carry: PermOwner,
			},
			expectedScript: `hears(".*dance.*", function()
	does("bounces lightly")
end)`,
		},
	}

	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			o := NewObject(0)
			err := o.SetScript(c.input)
			if err != nil && c.err == "" {
				t.Errorf("did not expect error but got: %s", err.Error())
				return
			} else if err != nil && c.err != "" {
				if err.Error() != c.err {
					t.Errorf("expected error '%s' but got '%s'", c.err, err.Error())
				}
				return
			} else if err == nil && c.err != "" {
				t.Errorf("expected error '%s' but got none", c.err)
				return
			}
			if o.GetData("foo") != "bar" || o.GetData("baz") != "quux" {
				t.Errorf("data not set correctly")
			}
			if !reflect.DeepEqual(c.expectedPerms, *o.Perms) {
				t.Errorf("got %v, expected %v", o.Perms, c.expectedPerms)
			}
			// TODO
		})
	}
}

func Test_parseScript(t *testing.T) {
	cs := []struct {
		name     string
		input    string
		expected parsedScript
		err      string
	}{
		{
			name: "basic",
			input: `--[[WITCH
data:
  foo: bar
  baz: quux
permissions:
  read: world
  write: owner
  exec: world
  carry: owner
--HCTIW]]

hears(".*dance.*", function()
	does("bounces lightly")
end)
`,
			expected: parsedScript{
				data: map[string]string{
					"foo": "bar",
					"baz": "quux",
				},
				perms: Permissions{
					Read:  PermWorld,
					Write: PermOwner,
					Exec:  PermWorld,
					Carry: PermOwner,
				},
				code: `
hears(".*dance.*", function()
	does("bounces lightly")
end)

`,
			},
		},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			result, err := parseScript(c.input)
			if err != nil && c.err == "" {
				t.Errorf("did not expect error but got: %s", err.Error())
				return
			} else if err != nil && c.err != "" {
				if err.Error() != c.err {
					t.Errorf("expected error '%s' but got '%s'", c.err, err.Error())
				}
				return
			} else if err == nil && c.err != "" {
				t.Errorf("expected error '%s' but got none", c.err)
				return
			}

			if !reflect.DeepEqual(c.expected.data, result.data) {
				t.Errorf("got %v, expected %v", result.data, c.expected.data)
			}

			if !reflect.DeepEqual(c.expected.perms, result.perms) {
				t.Errorf("got %v, expected %v", result.perms, c.expected.perms)
			}

			if c.expected.code != result.code {
				t.Errorf("got '%v', expected '%v'", result.code, c.expected.code)
			}
		})
	}
}
