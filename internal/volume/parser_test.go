package volume

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/mount"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/pack/testhelpers"
)

// These tests cover the parser vendored from moby (see VENDORED.txt). The
// corpora are taken from the upstream test suite so that the reduction applied
// while vendoring -- a trimmed MountPoint and stdlib regexp in place of moby's
// lazyregexp -- is held to upstream's own definition of correct behaviour.
//
// They run on every platform, unlike pack's processVolumes wrapper, because
// each parser is selected by the *image* OS rather than the host's.

func TestVolumeParsers(t *testing.T) {
	spec.Run(t, "volume_parsers", testVolumeParsers, spec.Parallel(), spec.Report(report.Terminal{}))
}

// mockFiProvider stands in for the filesystem so the Windows and LCOW parsers,
// which stat their source paths, can be exercised on any host. Mirrors the
// fixture upstream uses.
type mockFiProvider struct{}

func (mockFiProvider) fileInfo(path string) (exists, isDir bool, _ error) {
	dirs := map[string]struct{}{
		`c:\`:                    {},
		`c:\windows\`:            {},
		`c:\windows`:             {},
		`c:\program files`:       {},
		`c:\Windows`:             {},
		`c:\Program Files (x86)`: {},
		`\\?\c:\windows\`:        {},
	}
	files := map[string]struct{}{
		`c:\windows\system32\ntdll.dll`: {},
	}
	if _, ok := dirs[path]; ok {
		return true, true, nil
	}
	if _, ok := files[path]; ok {
		return true, false, nil
	}
	return false, false, nil
}

func testVolumeParsers(t *testing.T, when spec.G, it spec.S) {
	// assertParses runs a parser over a corpus of specs that must succeed and a
	// map of specs that must fail with a given error fragment.
	assertParses := func(t *testing.T, parser Parser, valid []string, invalid map[string]string) {
		t.Helper()
		for _, spec := range valid {
			mp, err := parser.ParseMountRaw(spec, "local")
			if err != nil {
				t.Fatalf("ParseMountRaw(%q) should have succeeded, got error: %s", spec, err)
			}
			h.AssertNotNil(t, mp)
		}
		for spec, wantErr := range invalid {
			mp, err := parser.ParseMountRaw(spec, "local")
			if err == nil {
				t.Fatalf("ParseMountRaw(%q) should have failed, got mount point: %+v", spec, mp)
			}
			if !strings.Contains(err.Error(), wantErr) {
				t.Fatalf("ParseMountRaw(%q) error should contain %q, got: %s", spec, wantErr, err)
			}
		}
	}

	when("#NewLinuxParser", func() {
		var parser Parser

		it.Before(func() {
			parser = NewLinuxParser()
			if p, ok := parser.(*linuxParser); ok {
				p.fi = mockFiProvider{}
			}
		})

		it("accepts valid volume specifications", func() {
			assertParses(t, parser, []string{
				"/home",
				"/home:/home",
				"/home:/something/else",
				"/with space",
				"/home:/with space",
				"relative:/absolute-path",
				"hostPath:/containerPath:ro",
				"/hostPath:/containerPath:rw",
				"/rw:/ro",
				"/hostPath:/containerPath:shared",
				"/hostPath:/containerPath:rshared",
				"/hostPath:/containerPath:slave",
				"/hostPath:/containerPath:rslave",
				"/hostPath:/containerPath:private",
				"/hostPath:/containerPath:rprivate",
				"/hostPath:/containerPath:ro,shared",
				"/hostPath:/containerPath:ro,slave",
				"/hostPath:/containerPath:ro,private",
				"/hostPath:/containerPath:ro,z,shared",
				"/hostPath:/containerPath:ro,Z,slave",
				"/hostPath:/containerPath:Z,ro,slave",
				"/hostPath:/containerPath:slave,Z,ro",
				"/hostPath:/containerPath:Z,slave,ro",
				"/hostPath:/containerPath:slave,ro,Z",
				"/hostPath:/containerPath:rslave,ro,Z",
				"/hostPath:/containerPath:ro,rshared,Z",
				"/hostPath:/containerPath:ro,Z,rprivate",
			}, nil)
		})

		it("rejects invalid volume specifications", func() {
			assertParses(t, parser, nil, map[string]string{
				"":                                "invalid volume specification",
				"./":                              "mount path must be absolute",
				"../":                             "mount path must be absolute",
				"/:../":                           "mount path must be absolute",
				"/:path":                          "mount path must be absolute",
				":":                               "invalid volume specification",
				"/tmp:":                           "invalid volume specification",
				":test":                           "invalid volume specification",
				":/test":                          "invalid volume specification",
				"tmp:":                            "invalid volume specification",
				":test:":                          "invalid volume specification",
				"::":                              "invalid volume specification",
				":::":                             "invalid volume specification",
				"/tmp:::":                         "invalid volume specification",
				":/tmp::":                         "invalid volume specification",
				"/path:rw":                        "invalid volume specification",
				"/path:ro":                        "invalid volume specification",
				"/rw:rw":                          "invalid volume specification",
				"path:ro":                         "invalid volume specification",
				"/path:/path:sw":                  "invalid mode",
				"/path:/path:rwz":                 "invalid mode",
				"/path:/path:ro,rshared,rslave":   "invalid mode",
				"/path:/path:ro,z,rshared,rslave": "invalid mode",
				"/path:shared":                    "invalid volume specification",
				"/path:slave":                     "invalid volume specification",
				"/path:private":                   "invalid volume specification",
				"name:/absolute-path:shared":      "invalid volume specification",
				"name:/absolute-path:rshared":     "invalid volume specification",
				"name:/absolute-path:slave":       "invalid volume specification",
				"name:/absolute-path:rslave":      "invalid volume specification",
				"name:/absolute-path:private":     "invalid volume specification",
				"name:/absolute-path:rprivate":    "invalid volume specification",
			})
		})

		// The fields asserted here are exactly the ones pack reads out of
		// MountPoint in processVolumes, so they are what the reduction in
		// mounts.go has to keep intact.
		it("populates the fields pack depends on", func() {
			mp, err := parser.ParseMountRaw("/hostPath:/containerPath:ro", "local")
			h.AssertNil(t, err)
			h.AssertEq(t, mp.Source, "/hostPath")
			h.AssertEq(t, mp.Destination, "/containerPath")
			h.AssertEq(t, mp.RW, false)
			h.AssertEq(t, mp.Mode, "ro")
			h.AssertEq(t, mp.Type, mount.TypeBind)
			h.AssertEq(t, mp.Spec.Source, "/hostPath")
			h.AssertEq(t, mp.Spec.Target, "/containerPath")
			h.AssertEq(t, mp.Spec.ReadOnly, true)
			h.AssertEq(t, mp.Propagation, parser.DefaultPropagationMode())
		})

		it("marks a spec without a mode as read-write", func() {
			mp, err := parser.ParseMountRaw("/hostPath:/containerPath", "local")
			h.AssertNil(t, err)
			h.AssertEq(t, mp.RW, true)
			h.AssertEq(t, mp.Mode, "")
			h.AssertEq(t, mp.Spec.ReadOnly, false)
		})

		it("treats a non-absolute source as a named volume", func() {
			mp, err := parser.ParseMountRaw("myvolume:/containerPath", "local")
			h.AssertNil(t, err)
			h.AssertEq(t, mp.Name, "myvolume")
			h.AssertEq(t, mp.Destination, "/containerPath")
			h.AssertEq(t, mp.Type, mount.TypeVolume)
			h.AssertEq(t, mp.Path(), "")
		})

		it("returns the source path from Path", func() {
			mp, err := parser.ParseMountRaw("/hostPath:/containerPath", "local")
			h.AssertNil(t, err)
			h.AssertEq(t, mp.Path(), "/hostPath")
		})

		it("reports read-write modes", func() {
			h.AssertEq(t, parser.ReadWrite("rw"), true)
			h.AssertEq(t, parser.ReadWrite(""), true)
			h.AssertEq(t, parser.ReadWrite("ro"), false)
		})

		// Upstream the Linux parser imposes no name restrictions at all; the
		// character rules are Windows-only. Asserted so a future re-vendor that
		// changes this is noticed rather than silently accepted.
		it("accepts any volume name", func() {
			h.AssertNil(t, parser.ValidateVolumeName("myvolume"))
			h.AssertNil(t, parser.ValidateVolumeName("a"))
			h.AssertNil(t, parser.ValidateVolumeName(`name<with>reserved:chars`))
		})
	})

	when("#NewWindowsParser", func() {
		var parser Parser

		it.Before(func() {
			parser = NewWindowsParser()
			if p, ok := parser.(*windowsParser); ok {
				p.fi = mockFiProvider{}
			}
		})

		it("accepts valid volume specifications", func() {
			assertParses(t, parser, []string{
				`d:\`,
				`d:`,
				`d:\path`,
				`d:\path with space`,
				`c:\:d:\`,
				`c:\windows\:d:`,
				`c:\windows:d:\s p a c e`,
				`c:\windows:d:\s p a c e:RW`,
				`c:\program files:d:\s p a c e i n h o s t d i r`,
				`0123456789name:d:`,
				`MiXeDcAsEnAmE:d:`,
				`test-aux-volume:d:`, // includes a reserved word, but is not one itself
				`name:D:`,
				`name:D::rW`,
				`name:D::RW`,
				`name:D::RO`,
				`c:/:d:/forward/slashes/are/good/too`,
				`c:/:d:/including with/spaces:ro`,
				`c:\Windows`,
				`c:\Program Files (x86)`,
				`\\?\c:\windows\:d:`,
				`c:\windows\:\\?\d:\`,
				`\\.\pipe\foo:\\.\pipe\foo`,
				`//./pipe/foo://./pipe/foo`,
			}, nil)
		})

		it("rejects invalid volume specifications", func() {
			assertParses(t, parser, nil, map[string]string{
				``:                                 "invalid volume specification: ",
				`.`:                                "invalid volume specification: ",
				`..\`:                              "invalid volume specification: ",
				`c:\:..\`:                          "invalid volume specification: ",
				`c:\:d:\:xyzzy`:                    "invalid volume specification: ",
				`c:`:                               "cannot be 'c:'",
				`c:\`:                              "cannot be 'c:'",
				`c:\notexist:d:`:                   `source path does not exist: c:\notexist`,
				`c:\windows\system32\ntdll.dll:d:`: "source path must be a directory",
				`name<:d:`:                         "invalid volume specification",
				`name>:d:`:                         "invalid volume specification",
				`name::d:`:                         "invalid volume specification",
				`name":d:`:                         "invalid volume specification",
				`name\:d:`:                         "invalid volume specification",
				`name*:d:`:                         "invalid volume specification",
				`name|:d:`:                         "invalid volume specification",
				`name?:d:`:                         "invalid volume specification",
				`name/:d:`:                         "invalid volume specification",
				`d:\pathandmode:rw`:                "invalid volume specification",
				`d:\pathandmode:ro`:                "invalid volume specification",
				`con:d:`:                           "cannot be a reserved word for Windows filenames",
				`PRN:d:`:                           "cannot be a reserved word for Windows filenames",
				`aUx:d:`:                           "cannot be a reserved word for Windows filenames",
				`nul:d:`:                           "cannot be a reserved word for Windows filenames",
				`com1:d:`:                          "cannot be a reserved word for Windows filenames",
				`lpt9:d:`:                          "cannot be a reserved word for Windows filenames",
				`c:\windows\system32\ntdll.dll`:    "Only directories can be mapped on this platform",
				`\\.\pipe\foo:c:\pipe`:             `'c:\pipe' is not a valid pipe path`,
			})
		})

		it("enforces the Windows volume name character rules", func() {
			h.AssertNil(t, parser.ValidateVolumeName("myvolume"))
			for _, name := range []string{`na\me`, `na/me`, `na:me`, `na*me`, `na?me`, `na"me`, `na<me`, `na>me`, `na|me`} {
				if err := parser.ValidateVolumeName(name); err == nil {
					t.Fatalf("ValidateVolumeName(%q) should have failed", name)
				}
			}
		})

		// ValidateVolumeName matches the reserved words case-sensitively; it is
		// the parse path that lowercases the name first, which is why the
		// corpus above rejects `PRN:d:` and `aUx:d:` while this does not.
		it("rejects reserved Windows filenames", func() {
			h.AssertNotNil(t, parser.ValidateVolumeName("con"))
			h.AssertNotNil(t, parser.ValidateVolumeName("lpt1"))
			h.AssertNil(t, parser.ValidateVolumeName("LPT1"))
		})
	})

	when("#NewLCOWParser", func() {
		var parser Parser

		it.Before(func() {
			parser = NewLCOWParser()
			if p, ok := parser.(*lcowParser); ok {
				p.fi = mockFiProvider{}
			}
		})

		it("accepts valid volume specifications", func() {
			assertParses(t, parser, []string{
				`/foo`,
				`/foo/`,
				`/foo bar`,
				`c:\:/foo`,
				`c:\windows\:/foo`,
				`c:\windows:/s p a c e`,
				`c:\windows:/s p a c e:RW`,
				`c:\program files:/s p a c e i n h o s t d i r`,
				`0123456789name:/foo`,
				`MiXeDcAsEnAmE:/foo`,
				`name:/foo`,
				`name:/foo:rW`,
				`name:/foo:RW`,
				`name:/foo:RO`,
				`c:/:/forward/slashes/are/good/too`,
				`c:/:/including with/spaces:ro`,
				`/Program Files (x86)`,
			}, nil)
		})

		it("rejects invalid volume specifications", func() {
			assertParses(t, parser, nil, map[string]string{
				``:                                   "invalid volume specification: ",
				`.`:                                  "invalid volume specification: ",
				`c:`:                                 "invalid volume specification: ",
				`c:\`:                                "invalid volume specification: ",
				`../`:                                "invalid volume specification: ",
				`c:\:../`:                            "invalid volume specification: ",
				`c:\:/foo:xyzzy`:                     "invalid volume specification: ",
				`/`:                                  "destination can't be '/'",
				`/..`:                                "destination can't be '/'",
				`c:\notexist:/foo`:                   `source path does not exist: c:\notexist`,
				`c:\windows\system32\ntdll.dll:/foo`: "source path must be a directory",
				`name<:/foo`:                         "invalid volume specification",
				`name::/foo`:                         "invalid volume specification",
				`/foo:rw`:                            "invalid volume specification",
				`/foo:ro`:                            "invalid volume specification",
				`con:/foo`:                           "cannot be a reserved word for Windows filenames",
				`lpt9:/foo`:                          "cannot be a reserved word for Windows filenames",
				`\\.\pipe\foo:/foo`:                  "containers on Windows do not support named pipe mounts",
			})
		})
	})
}
