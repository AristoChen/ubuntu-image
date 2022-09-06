// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/invopop/jsonschema"
	"github.com/snapcore/snapd/image"
	//"github.com/snapcore/snapd/osutil"
	//"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/store"
	"github.com/xeipuuv/gojsonschema"
)

// TestClassicSetup tests a successful run of the polymorphed Setup function
func TestClassicSetup(t *testing.T) {
	t.Run("test_classic_setup", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)
	})
}

// TestYAMLSchemaParsing attempts to parse a variety of image definition files, both
// valid and invalid, and ensures the correct result/errors are returned
func TestYAMLSchemaParsing(t *testing.T) {
	testCases := []struct {
		name            string
		imageDefinition string
		shouldPass      bool
		expectedError   string
	}{
		{"valid_image_definition", "test_valid.yaml", true, ""},
		{"invalid_class", "test_bad_class.yaml", false, "Class must be one of the following"},
		{"invalid_url", "test_bad_url.yaml", false, "Does not match format 'uri'"},
		{"invalid_ppa_name", "test_bad_ppa_name.yaml", false, "PPAName: Does not match pattern"},
		{"invalid_ppa_auth", "test_bad_ppa_name.yaml", false, "Auth: Does not match pattern"},
		{"both_seed_and_tasks", "test_both_seed_and_tasks.yaml", false, "Must validate one and only one schema"},
		{"git_gadget_without_url", "test_git_gadget_without_url.yaml", false, "When key gadget:type is specified as git, a URL must be provided"},
		{"file_doesnt_exist", "test_not_exist.yaml", false, "no such file or directory"},
		{"not_valid_yaml", "test_invalid_yaml.yaml", false, "yaml: unmarshal errors"},
		{"missing_yaml_fields", "test_missing_name.yaml", false, "Key \"name\" is required in struct \"ImageDefinition\", but is not in the YAML file!"},
		{"private_ppa_without_fingerprint", "test_private_ppa_without_fingerprint.yaml", false, "Fingerprint is required for private PPAs"},
		{"invalid_paths_in_manual_copy", "test_invalid_paths_in_manual_copy.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_copy_bug", "test_invalid_paths_in_manual_copy.yaml", false, "needs to be an absolute path (/../../malicious)"},
		{"invalid_paths_in_manual_touch_file", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_touch_file_bug", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (/../../malicious)"},
	}
	for _, tc := range testCases {
		t.Run("test_yaml_schema_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
				tc.imageDefinition)
			err := stateMachine.parseImageDefinition()

			if tc.shouldPass {
				asserter.AssertErrNil(err, false)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}
		})
	}
}

// TestFailedParseImageDefinition mocks function calls to test
// failure cases in the parseImageDefinition state
func TestFailedParseImageDefinition(t *testing.T) {
	t.Run("test_failed_parse_image_definition", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_valid.yaml")

		// mock helper.SetDefaults
		helperSetDefaults = mockSetDefaults
		defer func() {
			helperSetDefaults = helper.SetDefaults
		}()
		err := stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Test Error")
		helperSetDefaults = helper.SetDefaults

		// mock helper.CheckEmptyFields
		helperCheckEmptyFields = mockCheckEmptyFields
		defer func() {
			helperCheckEmptyFields = helper.CheckEmptyFields
		}()
		err = stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Test Error")
		helperCheckEmptyFields = helper.CheckEmptyFields

		// mock gojsonschema.Validate
		gojsonschemaValidate = mockGojsonschemaValidateError
		defer func() {
			gojsonschemaValidate = gojsonschema.Validate
		}()
		err = stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Schema validation returned an error")
		gojsonschemaValidate = gojsonschema.Validate
	})
}

// TestCalculateStates reads in a variety of yaml files and ensures
// that the correct states are added to the state machine
// TODO: manually assemble the image definitions instead of relying on the parseImageDefinition() function to make this more of a unit test
func TestCalculateStates(t *testing.T) {
	testCases := []struct {
		name            string
		imageDefinition string
		expectedStates  []string
	}{
		{"state_build_gadget", "test_build_gadget.yaml", []string{"build_gadget_tree", "load_gadget_yaml"}},
		{"state_prebuilt_gadget", "test_prebuilt_gadget.yaml", []string{"prepare_gadget_tree", "load_gadget_yaml"}},
		{"extract_rootfs_tar", "test_extract_rootfs_tar.yaml", []string{"extract_rootfs_tar"}},
		{"build_rootfs_from_seed", "test_rootfs_seed.yaml", []string{"germinate"}},
		{"build_rootfs_from_tasks", "test_rootfs_tasks.yaml", []string{"build_rootfs_from_tasks"}},
		{"customization_states", "test_customization.yaml", []string{"customize_cloud_init", "perform_manual_customization"}},
	}
	for _, tc := range testCases {
		t.Run("test_calcluate_states_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions", tc.imageDefinition)
			err := stateMachine.parseImageDefinition()
			asserter.AssertErrNil(err, true)

			// now calculate the states and ensure that the expected states are in the slice
			err = stateMachine.calculateStates()
			asserter.AssertErrNil(err, true)

			for _, expectedState := range tc.expectedStates {
				stateFound := false
				for _, state := range stateMachine.states {
					if expectedState == state.name {
						stateFound = true
					}
				}
				if !stateFound {
					t.Errorf("state %s should exist in %v, but does not",
						expectedState, stateMachine.states)
				}
			}
		})
	}
}

// TestFailedCalculateStates tests that the calculateStates
// function fails if the value of --until or --thru is not
// in the calculated list of states
func TestFailedCalculateStates(t *testing.T) {
	t.Run("test_failed_calcluate_states", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Gadget: &GadgetType{
				GadgetType: "git",
			},
			Rootfs: &RootfsType{
				ArchiveTasks: []string{"test"},
			},
			Customization: &CustomizationType{},
		}

		stateMachine.stateMachineFlags.Thru = "fake_state"

		// now calculate the states and ensure that the expected states are in the slice
		err := stateMachine.calculateStates()
		asserter.AssertErrContains(err, "not a valid state name")
	})
}

// TestPrintStates ensures the states are printed to stdout when the --debug flag is set
func TestPrintStates(t *testing.T) {
	t.Run("test_print_states", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.Debug = true
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions", "test_valid.yaml")
		err := stateMachine.parseImageDefinition()
		asserter.AssertErrNil(err, true)

		// capture stdout, calculate the states, and ensure they were printed
		stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
		defer restoreStdout()
		asserter.AssertErrNil(err, true)

		err = stateMachine.calculateStates()
		asserter.AssertErrNil(err, true)

		// restore stdout and examine what was printed
		restoreStdout()
		readStdout, err := ioutil.ReadAll(stdout)
		asserter.AssertErrNil(err, true)

		expectedStates := `The calculated states are as follows:
[0] build_gadget_tree
[1] prepare_gadget_tree
[2] load_gadget_yaml
[3] germinate
[4] create_chroot
[5] install_packages
[6] preseed_image
[7] customize_cloud_init
[8] customize_fstab
[9] perform_manual_customization
[10] populate_rootfs_contents
[11] generate_disk_info
[12] calculate_rootfs_size
[13] populate_bootfs_contents
[14] populate_prepare_partitions
[15] make_disk
[16] generate_manifest
[17] finish
`
		if !strings.Contains(string(readStdout), expectedStates) {
			t.Errorf("Expected states to be printed in output:\n\"%s\"\n but got \n\"%s\"\n instead",
				expectedStates, string(readStdout))
		}
	})
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "cannot specify both --until and --thru")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "error reading metadata file")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepareGadgetTree runs prepareGadgetTree() and ensures the gadget_tree files
// are placed in the correct locations
func TestPrepareGadgetTree(t *testing.T) {
	t.Run("test_prepare_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.prepareGadgetTree()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPrepareGadgetTree tests failures in os, osutil, and ioutil libraries
func TestFailedPrepareGadgetTree(t *testing.T) {
	t.Run("test_failed_prepare_gadget_tree", func(t *testing.T) {
		// currently a no-op, waiting for prepareGadgetTree
		// to be converted to the new ubuntu-image classic
		// design. This will have ubuntu-image build the
		// gadget tree rather than relying on the user
		// to have done this ahead of time
		t.Skip()
	})
}

// TestBuildRootfsFromTasks unit tests the buildRootfsFromTasks function
func TestBuildRootfsFromTasks(t *testing.T) {
	t.Run("test_build_rootfs_from_tasks", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.buildRootfsFromTasks()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestExtractRootfsTar unit tests the extractRootfsTar function
func TestExtractRootfsTar(t *testing.T) {
	t.Run("test_extract_rootfs_tar", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.extractRootfsTar()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCustomizeCloudInit unit tests the customizeCloudInit function
func TestCustomizeCloudInit(t *testing.T) {
	t.Run("test_customize_cloud_init", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.customizeCloudInit()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestSetupExtraPPAs unit tests the setupExtraPPAs function
func TestSetupExtraPPAs(t *testing.T) {
	t.Run("test_setup_extra_PPAs", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.setupExtraPPAs()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestInstallExtraPackages unit tests the installExtraPackages function
func TestInstallExtraPackages(t *testing.T) {
	t.Run("test_install_extra_packages", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.installExtraPackages()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestManualCustomization unit tests the manualCustomization function
func TestManualCustomization(t *testing.T) {
	t.Run("test_manual_customization", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs: &RootfsType{
				Archive: "ubuntu",
			},
			Customization: &CustomizationType{
				Manual: &ManualType{
					CopyFile: []*CopyFileType{
						{
							Source: filepath.Join("testdata", "test_script"),
							Dest:   "/test_copy_file",
						},
					},
					TouchFile: []*TouchFileType{
						{
							TouchPath: "/test_touch_file",
						},
					},
					Execute: []*ExecuteType{
						{
							// the file we already copied creates a file /test_execute
							ExecutePath: "/test_copy_file",
						},
					},
					AddUser: []*AddUserType{
						{
							UserName: "testuser",
							UserID:   "123456",
						},
					},
					AddGroup: []*AddGroupType{
						{
							GroupName: "testgroup",
							GroupID:   "456789",
						},
					},
				},
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// also create chroot
		err = stateMachine.createChroot()
		asserter.AssertErrNil(err, true)

		err = stateMachine.manualCustomization()
		asserter.AssertErrNil(err, true)

		// Check that the correct files exist
		testFiles := []string{"test_copy_file", "test_touch_file", "test_execute"}
		for _, fileName := range testFiles {
			_, err := os.Stat(filepath.Join(stateMachine.tempDirs.chroot, fileName))
			if err != nil {
				t.Errorf("file %s should exist, but it does not", fileName)
			}
		}

		// Check that the test user exists with the correct uid
		passwdFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "passwd")
		passwdContents, err := ioutil.ReadFile(passwdFile)
		asserter.AssertErrNil(err, true)
		if !strings.Contains(string(passwdContents), "testuser:x:123456") {
			t.Errorf("Test user was not created in the chroot")
		}

		// Check that the test group exists with the correct gid
		groupFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "group")
		groupContents, err := ioutil.ReadFile(groupFile)
		asserter.AssertErrNil(err, true)
		if !strings.Contains(string(groupContents), "testgroup:x:456789") {
			t.Errorf("Test group was not created in the chroot")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedManualCustomization tests failures in the manualCustomization function
func TestFailedManualCustomization(t *testing.T) {
	t.Run("test_failed_manual_customization", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		stateMachine.ImageDef = ImageDefinition{
			Customization: &CustomizationType{
				Manual: &ManualType{
					TouchFile: []*TouchFileType{
						{
							TouchPath: filepath.Join("this", "path", "does", "not", "exist"),
						},
					},
				},
			},
		}

		err := stateMachine.manualCustomization()
		asserter.AssertErrContains(err, "no such file or directory")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPreseedClassicImage unit tests the preseedClassicImage function
func TestPreseedClassicImage(t *testing.T) {
	t.Run("test_preseed_classic_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Snaps = []string{"lxd"}
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Customization: &CustomizationType{
				ExtraSnaps: []*SnapType{
					{
						SnapName: "hello",
						Channel:  "candidate",
					},
				},
			},
		}

		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.preseedClassicImage()
		asserter.AssertErrNil(err, true)

		// check that the lxd and hello snaps, as well as lxd's base, core20
		// were preseeded in the correct location
		snaps := map[string]string{"lxd": "stable", "hello": "candidate", "core20": "stable"}
		for snapName, snapChannel := range snaps {
			// reach out to the snap store to find the revision
			// of the snap for the specified channel
			snapStore := store.New(nil, nil)
			snapSpec := store.SnapSpec{Name: snapName}
			context := context.TODO() //context can be empty, just not nil
			snapInfo, err := snapStore.SnapInfo(context, snapSpec, nil)
			asserter.AssertErrNil(err, true)

			var storeRevision int
			storeRevision = snapInfo.Channels["latest/"+snapChannel].Revision.N
			snapFileName := fmt.Sprintf("%s_%d.snap", snapName, storeRevision)

			snapPath := filepath.Join(stateMachine.tempDirs.chroot,
				"var", "lib", "snapd", "seed", "snaps", snapFileName)
			_, err = os.Stat(snapPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Errorf("File %s should exist, but does not", snapPath)
				}
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPreseedClassicImage tests failures in the preseedClassicImage function
func TestFailedPreseedClassicImage(t *testing.T) {
	t.Run("test_failed_preseed_classic_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Customization: &CustomizationType{
				ExtraSnaps: []*SnapType{},
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// include an invalid snap snap name to trigger a failure in
		// parseSnapsAndChannels
		stateMachine.Snaps = []string{"lxd=test=invalid=name"}
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Invalid syntax")

		// try to include a nonexistent snap to trigger a failure
		// in snapStore.SnapInfo
		stateMachine.Snaps = []string{"test-this-snap-name-should-never-exist"}
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Error getting info for snap")

		// mock image.Prepare
		stateMachine.Snaps = []string{"hello"}
		imagePrepare = mockImagePrepare
		defer func() {
			imagePrepare = image.Prepare
		}()
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Error preparing image")
		imagePrepare = image.Prepare

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TODO replace this with fakeExecCommand that sil2100 wrote
// TestFailedLiveBuildCommands tests the scenario where calls to `lb` fail
// this is accomplished by temporarily replacing lb on disk with a test script
/*func TestFailedLiveBuildCommands(t *testing.T) {
	testCases := []struct {
		name       string
		testScript string
	}{
		{"failed_lb_config", "lb_config_fail"},
		{"failed_lb_build", "lb_build_fail"},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.Opts.Project = "ubuntu-cpc"
			stateMachine.Opts.Subproject = "fakeproject"
			stateMachine.Opts.Subarch = "fakearch"
			stateMachine.Opts.WithProposed = true
			stateMachine.Opts.ExtraPPAs = []string{"ppa:fake_user/fakeppa"}
			stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
			stateMachine.parent = &stateMachine

			scriptPath := filepath.Join("testscripts", tc.testScript)
			// save the original lb
			whichLb := *exec.Command("which", "lb")
			lbLocationBytes, _ := whichLb.Output()
			lbLocation := strings.TrimSpace(string(lbLocationBytes))
			// ensure the backup doesn't exist
			os.Remove(lbLocation + ".bak")
			err := os.Rename(lbLocation, lbLocation+".bak")
			asserter.AssertErrNil(err, true)

			err = osutil.CopyFile(scriptPath, lbLocation, 0)
			asserter.AssertErrNil(err, true)
			defer func() {
				os.Remove(lbLocation)
				os.Rename(lbLocation+".bak", lbLocation)
			}()

			// need workdir set up for this
			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// also need unpack set up
			err = os.Mkdir(stateMachine.tempDirs.unpack, 0755)
			asserter.AssertErrNil(err, true)

			err = stateMachine.runLiveBuild()
			asserter.AssertErrContains(err, "Error running command")
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}*/

// TestPopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_populate_classic_rootfs_contents", func(t *testing.T) {
		if runtime.GOARCH != "amd64" {
			t.Skip("Test for amd64 only")
		}
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_valid.yaml")

		err := stateMachine.populateClassicRootfsContents()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// check the files before Teardown
		/*fileList := []string{filepath.Join("etc", "shadow"),
			filepath.Join("etc", "systemd"),
			filepath.Join("boot", "vmlinuz"),
			filepath.Join("boot", "grub"),
			filepath.Join("usr", "lib")}
		for _, file := range fileList {
			_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
			if err != nil {
				if os.IsNotExist(err) {
					t.Errorf("File %s should exist, but does not", file)
				}
			}
		}

		// check /etc/fstab contents to test the scenario where the regex replaced an
		// existing filesystem label with LABEL=writable
		fstab, err := ioutilReadFile(filepath.Join(stateMachine.tempDirs.rootfs,
			"etc", "fstab"))
		if err != nil {
			t.Errorf("Error reading fstab to check regex")
		}
		correctLabel := "LABEL=writable"
		if !strings.Contains(string(fstab), correctLabel) {
			t.Errorf("Expected fstab contents %s to contain %s",
				string(fstab), correctLabel)
		}

		// check that extra snaps were added to the rootfs
		for _, snap := range stateMachine.commonFlags.Snaps {
			if strings.Contains(snap, "/") {
				snap = strings.Split(snap, "/")[0]
			}
			if strings.Contains(snap, "=") {
				snap = strings.Split(snap, "=")[0]
			}
			filePath := filepath.Join(stateMachine.tempDirs.rootfs,
				"var", "snap", snap)
			if !osutil.FileExists(filePath) {
				t.Errorf("File %s should exist but it does not", filePath)
			}
		}*/
	})
}

// TestFailedPopulateClassicRootfsContents tests failed scenarios in populateClassicRootfsContents
// this is accomplished by mocking functions
/*func TestFailedPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_classic_rootfs_contents", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")
		stateMachine.commonFlags.CloudInit = filepath.Join("testdata", "user-data")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error reading unpack/chroot dir")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error copying rootfs")
		osutilCopySpecialFile = osutil.CopySpecialFile

		// mock ioutil.WriteFile
		ioutilWriteFile = mockWriteFile
		defer func() {
			ioutilWriteFile = ioutil.WriteFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error writing to fstab")
		ioutilWriteFile = ioutil.WriteFile

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error creating cloud-init dir")
		osMkdirAll = os.MkdirAll

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error opening cloud-init meta-data file")
		osOpenFile = os.OpenFile

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error copying cloud-init")
		osutilCopyFile = osutil.CopyFile
	})
}

// TestFilesystemFlag makes sure that with the --filesystem flag the specified filesystem is copied
// to the rootfs directory
func TestFilesystemFlag(t *testing.T) {
	t.Run("test_filesystem_flag", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrNil(err, true)

		// check that the specified filesystem was copied over
		if _, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, "testfile")); err != nil {
			t.Errorf("Failed to copy --filesystem to rootfs")
		}

		// the included filesystem contains an invalid /etc/fstab. Make sure it
		// was overwritten to have a valid /etc/fstab
		fstab, err := ioutilReadFile(filepath.Join(stateMachine.tempDirs.rootfs,
			"etc", "fstab"))
		if err != nil {
			t.Errorf("Error reading fstab to check regex")
		}
		correctLabel := "LABEL=writable   /    ext4   defaults    0 0"
		if !strings.Contains(string(fstab), correctLabel) {
			t.Errorf("Expected fstab contents %s to contain %s",
				string(fstab), correctLabel)
		}
	})
}
*/
// TestGeneratePackageManifest tests if classic image manifest generation works
func TestGeneratePackageManifest(t *testing.T) {
	t.Run("test_generate_package_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// We need the output directory set for this
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.OutputDir = outputDir
		osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)

		err = stateMachine.generatePackageManifest()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		// Check if manifest file got generated and if it has expected contents
		/*manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.manifest")
		manifestBytes, err := ioutil.ReadFile(manifestPath)
		asserter.AssertErrNil(err, true)
		// The order of packages shouldn't matter
		examplePackages := []string{"foo 1.2", "bar 1.4-1ubuntu4.1", "libbaz 0.1.3ubuntu2"}
		for _, pkg := range examplePackages {
			if !strings.Contains(string(manifestBytes), pkg) {
				t.Errorf("filesystem.manifest does not contain expected package: %s", pkg)
			}
		}*/
	})
}

/*
// TestFailedGeneratePackageManifest tests if classic manifest generation failures are reported
func TestFailedGeneratePackageManifest(t *testing.T) {
	t.Run("test_failed_generate_package_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock - version from the success test
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// Setup the mock for os.Create, making those fail
		osCreate = mockCreate
		defer func() {
			osCreate = os.Create
		}()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.OutputDir = "/test/path"

		err := stateMachine.generatePackageManifest()
		asserter.AssertErrContains(err, "Error creating manifest file")
	})
}

// TestExtraSnapsWithFilesystem tests that using --snap along with --filesystem preseeds the snaps
// in the resulting root filesystem
func TestExtraSnapsWithFilesystem(t *testing.T) {
	t.Run("test_extra_snaps_with_filesystem", func(t *testing.T) {
		if runtime.GOARCH != "amd64" {
			t.Skip("Test for amd64 only")
		}
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")
		stateMachine.commonFlags.Snaps = []string{"hello"}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// copy the filesystem over before attempting to preseed it
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrNil(err, true)

		// call "snap prepare image" to preseed the filesystem.
		// Doing the preseed at the time of the test allows it to
		// run on each architecture and keeps the github repository
		// free of large .snap files
		snapPrepareImage := *exec.Command("snap", "prepare-image", "--arch=amd64",
			"--classic", "--snap=core20", "--snap=snapd", "--snap=lxd",
			filepath.Join("testdata", "modelAssertionClassic"),
			stateMachine.tempDirs.rootfs)
		err = snapPrepareImage.Run()
		asserter.AssertErrNil(err, true)

		// now call prepateClassicImage to simulate using --snap with --filesystem
		err = stateMachine.prepareClassicImage()
		asserter.AssertErrNil(err, true)

		// Ensure that the hello snap was preseeded in the filesystem and the
		// snaps that were already there haven't been removed
		snapList := []string{"hello", "lxd", "core20", "snapd"}
		for _, snap := range snapList {
			snapGlob := filepath.Join(stateMachine.tempDirs.rootfs,
				"var", "lib", "snapd", "snaps", snap+"*.snap")
			snapFile, _ := filepath.Glob(snapGlob)
			if len(snapFile) == 0 {
				if os.IsNotExist(err) {
					t.Errorf("File %s should exist, but does not", snapGlob)
				}
			}
		}
	})
}

// TestFailedPrepareClassiImage tests various failure scenarios in the prepateClassicImage function
func TestFailedPrepareClassicImage(t *testing.T) {
	t.Run("test_failed_prepare_classic_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// copy the filesystem over before attempting to preseed it
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrNil(err, true)

		// call "snap prepare image" to preseed the filesystem.
		// Doing the preseed at the time of the test allows it to
		// run on each architecture and keeps the github repository
		// free of large .snap files
		snapPrepareImage := *exec.Command("snap", "prepare-image", "--arch=amd64",
			"--classic", "--snap=core20", "--snap=snapd", "--snap=lxd",
			filepath.Join("testdata", "modelAssertionClassic"),
			stateMachine.tempDirs.rootfs)
		err = snapPrepareImage.Run()
		asserter.AssertErrNil(err, true)

		// set an invalid value for --snap to cause an error in
		// parseSnapsAndChannels
		stateMachine.commonFlags.Snaps = []string{"hello=test=invalid"}
		err = stateMachine.prepareClassicImage()
		asserter.AssertErrContains(err, "Invalid syntax passed to --snap")
		os.RemoveAll(filepath.Join(stateMachine.stateMachineFlags.WorkDir, "model"))

		// set a valid value for --snap and mock seed.Open to simulate
		// a failure reading the seed
		stateMachine.commonFlags.Snaps = []string{"hello"}
		seedOpen = mockSeedOpen
		defer func() {
			seedOpen = seed.Open
		}()
		err = stateMachine.prepareClassicImage()
		asserter.AssertErrContains(err, "Error removing preseeded snaps")
		seedOpen = seed.Open
		os.RemoveAll(filepath.Join(stateMachine.stateMachineFlags.WorkDir, "model"))

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.prepareClassicImage()
		asserter.AssertErrContains(err, "Error copying model")
		osutilCopyFile = osutil.CopyFile
		os.RemoveAll(filepath.Join(stateMachine.stateMachineFlags.WorkDir, "model"))

		// mock image.Prepare
		stateMachine.commonFlags.Snaps = []string{"hello"}
		imagePrepare = mockImagePrepare
		defer func() {
			imagePrepare = image.Prepare
		}()
		err = stateMachine.prepareClassicImage()
		asserter.AssertErrContains(err, "Error preparing image")
		imagePrepare = image.Prepare

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}*/

// TestSuccessfulClassicRun runs through a full classic state machine run and ensures
// it is successful
func TestSuccessfulClassicRun(t *testing.T) {
	t.Run("test_successful_classic_run", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.Debug = true
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_amd64.yaml")

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		// make sure packages were successfully installed from public and private ppas
		files := []string{
			filepath.Join(stateMachine.tempDirs.chroot, "usr", "bin", "hello-ubuntu-image-public"),
			filepath.Join(stateMachine.tempDirs.chroot, "usr", "bin", "hello-ubuntu-image-private"),
		}
		for _, file := range files {
			_, err = os.Stat(file)
			asserter.AssertErrNil(err, true)
		}

		// make sure snaps from the correct channel were installed
		type snapList struct {
			Snaps []struct {
				Name    string `yaml:"name"`
				Channel string `yaml:"channel"`
			} `yaml:"snaps"`
		}

		seedYaml := filepath.Join(stateMachine.tempDirs.chroot,
			"var", "lib", "snapd", "seed", "seed.yaml")

		seedFile, err := os.Open(seedYaml)
		defer seedFile.Close()
		asserter.AssertErrNil(err, true)

		var seededSnaps snapList
		err = yaml.NewDecoder(seedFile).Decode(&seededSnaps)
		asserter.AssertErrNil(err, true)

		expectedSnapChannels := map[string]string{
			"hello":  "candidate",
			"core20": "stable",
		}

		for _, seededSnap := range seededSnaps.Snaps {
			channel, found := expectedSnapChannels[seededSnap.Name]
			if found {
				if channel != seededSnap.Channel {
					t.Errorf("Expected snap %s to be pre-seeded with channel %s, but got %s",
						seededSnap.Name, channel, seededSnap.Channel)
				}
			}
		}

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCheckEmptyFields unit tests the helper.CheckEmptyFields function
func TestCheckEmptyFields(t *testing.T) {
	// define the struct we will use to test
	type testStruct struct {
		A string `yaml:"a" json:"fieldA,required"`
		B string `yaml:"b" json:"fieldB"`
		C string `yaml:"c" json:"fieldC,omitempty"`
	}

	// generate the schema for our testStruct
	var jsonReflector jsonschema.Reflector
	schema := jsonReflector.Reflect(&testStruct{})

	// now run CheckEmptyFields with a variety of test data
	// to ensure the correct return values
	testCases := []struct {
		name       string
		structData testStruct
		shouldPass bool
	}{
		{"success", testStruct{A: "foo", B: "bar", C: "baz"}, true},
		{"missing_explicitly_required", testStruct{B: "bar", C: "baz"}, false},
		{"missing_implicitly_required", testStruct{A: "foo", C: "baz"}, false},
		{"missing_omitempty", testStruct{A: "foo", B: "bar"}, true},
	}
	for _, tc := range testCases {
		t.Run("test_check_empty_fields_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			result := new(gojsonschema.Result)
			err := helper.CheckEmptyFields(&tc.structData, result, schema)
			asserter.AssertErrNil(err, false)
			schema.Required = append(schema.Required, "fieldA")

			// make sure validation will fail only when expected
			if tc.shouldPass && !result.Valid() {
				t.Error("CheckEmptyFields added errors when it should not have")
			}
			if !tc.shouldPass && result.Valid() {
				t.Error("CheckEmptyFields did NOT add errors when it should have")
			}

		})
	}
}

// TestGerminate tests the germinate state and ensures some necessary packages are included
func TestGerminate(t *testing.T) {
	testCases := []struct {
		name             string
		flavor           string
		seedURLs         []string
		seedNames        []string
		expectedPackages []string
		expectedSnaps    []string
		vcs              bool
	}{
		{
			"git",
			"ubuntu",
			[]string{"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"},
			[]string{"server", "minimal", "standard", "cloud-image"},
			[]string{"python3", "sudo", "cloud-init", "ubuntu-server"},
			[]string{"lxd"},
			true,
		},
		{
			"http",
			"ubuntu",
			[]string{"https://people.canonical.com/~ubuntu-archive/seeds/"},
			[]string{"server", "minimal", "standard", "cloud-image"},
			[]string{"python3", "sudo", "cloud-init", "ubuntu-server"},
			[]string{"lxd"},
			false,
		},
		{
			"bzr+git",
			"ubuntu",
			[]string{"http://bazaar.launchpad.net/~ubuntu-mate-dev/ubuntu-seeds/",
				"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/",
				"https://people.canonical.com/~ubuntu-archive/seeds/",
			},
			[]string{"desktop", "desktop-common", "standard", "minimal"},
			[]string{"xorg", "wget", "ubuntu-minimal"},
			[]string{},
			true,
		},
	}
	for _, tc := range testCases {
		t.Run("test_germinate_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			hostArch := getHostArch()
			hostSuite := getHostSuite()
			imageDef := ImageDefinition{
				Architecture: hostArch,
				Series:       hostSuite,
				Rootfs: &RootfsType{
					Flavor: tc.flavor,
					Mirror: "http://archive.ubuntu.com/ubuntu/",
					Seed: &SeedType{
						SeedURLs:   tc.seedURLs,
						SeedBranch: hostSuite,
						Names:      tc.seedNames,
						Vcs:        tc.vcs,
					},
				},
			}

			stateMachine.ImageDef = imageDef

			err = stateMachine.germinate()
			asserter.AssertErrNil(err, true)

			// spot check some packages that should remain seeded for a long time
			for _, expectedPackage := range tc.expectedPackages {
				found := false
				for _, seedPackage := range stateMachine.Packages {
					if expectedPackage == seedPackage {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected to find %s in list of packages: %v",
						expectedPackage, stateMachine.Packages)
				}
			}
			// spot check some snaps that should remain seeded for a long time
			for _, expectedSnap := range tc.expectedSnaps {
				found := false
				for _, seedSnap := range stateMachine.Snaps {
					snapName := strings.Split(seedSnap, "=")[0]
					if expectedSnap == snapName {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected to find %s in list of snaps: %v",
						expectedSnap, stateMachine.Snaps)
				}
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedGerminate mocks function calls to test
// failure cases in the germinate state
func TestFailedGerminate(t *testing.T) {
	t.Run("test_failed_germinate", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// create a valid imageDefinition
		hostArch := getHostArch()
		hostSuite := getHostSuite()
		imageDef := ImageDefinition{
			Architecture: hostArch,
			Series:       hostSuite,
			Rootfs: &RootfsType{
				Flavor: "ubuntu",
				Mirror: "http://archive.ubuntu.com/ubuntu/",
				Seed: &SeedType{
					SeedURLs:   []string{"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"},
					SeedBranch: hostSuite,
					Names:      []string{"server", "minimal", "standard", "cloud-image"},
					Vcs:        true,
				},
			},
		}
		stateMachine.ImageDef = imageDef

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error creating germinate directory")
		osMkdir = os.Mkdir

		// Setup the exec.Command mock
		testCaseName = "TestFailedGerminate"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error running germinate command")
		execCommand = exec.Command

		// mock os.Open
		osOpen = mockOpen
		defer func() {
			osOpen = os.Open
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error opening seed file")
		osOpen = os.Open

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestBuildGadgetTree tests the successful build of a gadget tree
func TestBuildGadgetTree(t *testing.T) {
	t.Run("test_build_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// test the directory method
		wd, _ := os.Getwd()
		sourcePath := filepath.Join(wd, "testdata", "gadget_source")
		sourcePath = "file://" + sourcePath
		imageDef := ImageDefinition{
			Gadget: &GadgetType{
				GadgetURL:  sourcePath,
				GadgetType: "directory",
			},
		}

		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrNil(err, true)

		// test the git method
		imageDef = ImageDefinition{
			Gadget: &GadgetType{
				GadgetURL:    "https://github.com/snapcore/pc-amd64-gadget",
				GadgetType:   "git",
				GadgetBranch: "classic",
			},
		}

		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedBuildGadgetTree tests failures in the  buildGadgetTree function
func TestFailedBuildGadgetTree(t *testing.T) {
	t.Run("test_failed_build_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.MkdirAll
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.buildGadgetTree()
		asserter.AssertErrContains(err, "Error creating scratch/gadget")
		osMkdir = os.Mkdir

		// try to clone a repo that doesn't exist
		imageDef := ImageDefinition{
			Gadget: &GadgetType{
				GadgetURL:  "http://fakerepo.git",
				GadgetType: "git",
			},
		}
		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrContains(err, "Error cloning gadget repository")

		// try to copy a file that doesn't exist
		imageDef = ImageDefinition{
			Gadget: &GadgetType{
				GadgetURL:  "file:///fake/file/that/does/not/exist",
				GadgetType: "directory",
			},
		}
		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrContains(err, "Error copying gadget source")

		// run a "make" command that will fail by mocking exec.Command
		testCaseName = "TestFailedBuildGadgetTree"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		wd, _ := os.Getwd()
		sourcePath := filepath.Join(wd, "testdata", "gadget_source")
		sourcePath = "file://" + sourcePath
		imageDef = ImageDefinition{
			Gadget: &GadgetType{
				GadgetURL:  sourcePath,
				GadgetType: "directory",
			},
		}
		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrContains(err, "Error running \"make\" in gadget source")

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCreateChroot runs the createChroot step and spot checks that some
// expected files in the chroot exist
func TestCreateChroot(t *testing.T) {
	t.Run("test_create_chroot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs: &RootfsType{
				Pocket: "proposed",
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.createChroot()
		asserter.AssertErrNil(err, true)

		expectedFiles := []string{
			"etc",
			"home",
			"boot",
			"var",
		}
		for _, expectedFile := range expectedFiles {
			fullPath := filepath.Join(stateMachine.tempDirs.chroot, expectedFile)
			_, err := os.Stat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Errorf("File \"%s\" should exist, but does not", fullPath)
				}
			}
		}

		// check that security, updates, and proposed were added to /etc/apt/sources.list
		sourcesList := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list")
		sourcesListData, err := os.ReadFile(sourcesList)
		asserter.AssertErrNil(err, true)

		pockets := []string{
			fmt.Sprintf("%s-updates", stateMachine.ImageDef.Series),
			fmt.Sprintf("%s-security", stateMachine.ImageDef.Series),
			fmt.Sprintf("%s-proposed", stateMachine.ImageDef.Series),
		}

		for _, pocket := range pockets {
			if !strings.Contains(string(sourcesListData), pocket) {
				t.Errorf("%s is not present in /etc/apt/sources.list", pocket)
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedCreateChroot tests failure cases in createChroot
func TestFailedCreateChroot(t *testing.T) {
	t.Run("test_failed_create_chroot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &RootfsType{},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.createChroot()
		asserter.AssertErrContains(err, "Failed to create chroot")
		osMkdir = os.Mkdir

		// Setup the exec.Command mock
		testCaseName = "TestFailedCreateChroot"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.createChroot()
		asserter.AssertErrContains(err, "Error running debootstrap command")
		execCommand = exec.Command

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedInstallPackages tests failure cases in installPackages
func TestFailedInstallPackages(t *testing.T) {
	t.Run("test_failed_install_packages", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &RootfsType{},
			Customization: &CustomizationType{
				ExtraPackages: []*PackageType{
					{
						PackageName: "test1",
					},
				},
			},
		}

		// Setup the exec.Command mock
		testCaseName = "TestFailedInstallPackages"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err := stateMachine.installPackages()
		asserter.AssertErrContains(err, "Error running command")
		execCommand = exec.Command

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedAddExtraPPAs tests failure cases in addExtraPPAs
func TestFailedAddExtraPPAs(t *testing.T) {
	t.Run("test_failed_add_extra_ppas", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		validPPA := &PPAType{
			PPAName: "canonical-foundations/ubuntu-image",
		}
		invalidPPA := &PPAType{
			PPAName:     "canonical-foundations/ubuntu-image",
			Fingerprint: "TEST FINGERPRINT",
		}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &RootfsType{},
			Customization: &CustomizationType{
				ExtraPPAs: []*PPAType{
					validPPA,
				},
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// create the /etc/apt/ dir in workdir
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "trusted.gpg.d"), 0755)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Failed to create apt sources.list.d")
		osMkdir = os.Mkdir

		// mock os.MkdirTemp
		osMkdirTemp = mockMkdirTemp
		defer func() {
			osMkdirTemp = os.MkdirTemp
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error creating temp dir for gpg")
		osMkdirTemp = os.MkdirTemp

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error creating")
		osOpenFile = os.OpenFile

		// Use an invalid PPA to trigger a failure in importPPAKeys
		stateMachine.ImageDef.Customization.ExtraPPAs = []*PPAType{invalidPPA}
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error retrieving signing key")
		stateMachine.ImageDef.Customization.ExtraPPAs = []*PPAType{validPPA}

		// mock os.RemoveAll
		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error removing temporary gpg directory")
		osRemoveAll = os.RemoveAll

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCustomizeFstab tests functionality of the customizeFstab function
func TestCustomizeFstab(t *testing.T) {
	testCases := []struct {
		name          string
		fstab         []*FstabType
		expectedFstab string
	}{
		{
			"one_entry",
			[]*FstabType{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         true,
					FsckOrder:    1,
				},
			},
			`LABEL=writable	/	ext4	defaults	1	1`,
		},
		{
			"two_entries",
			[]*FstabType{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         false,
					FsckOrder:    1,
				},
				{
					Label:        "system-boot",
					Mountpoint:   "/boot/firmware",
					FSType:       "vfat",
					MountOptions: "defaults",
					Dump:         false,
					FsckOrder:    1,
				},
			},
			`LABEL=writable	/	ext4	defaults	0	1
LABEL=system-boot	/boot/firmware	vfat	defaults	0	1`,
		},
		{
			"defaults_assumed",
			[]*FstabType{
				{
					Label:      "writable",
					Mountpoint: "/",
					FSType:     "ext4",
					FsckOrder:  1,
				},
			},
			`LABEL=writable	/	ext4	defaults	0	1`,
		},
	}

	for _, tc := range testCases {
		t.Run("test_customize_fstab_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Rootfs:       &RootfsType{},
				Customization: &CustomizationType{
					Fstab: tc.fstab,
				},
			}

			// set the defaults for the imageDef
			err := helper.SetDefaults(&stateMachine.ImageDef)
			asserter.AssertErrNil(err, true)

			// need workdir set up for this
			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// create the <chroot>/etc directory
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0644)
			asserter.AssertErrNil(err, true)

			// customize the fstab, ensure no errors, and check the contents
			err = stateMachine.customizeFstab()
			asserter.AssertErrNil(err, true)

			fstabBytes, err := ioutil.ReadFile(
				filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab"),
			)
			asserter.AssertErrNil(err, true)

			if string(fstabBytes) != tc.expectedFstab {
				t.Errorf("Expected fstab contents \"%s\", but got \"%s\"",
					tc.expectedFstab, string(fstabBytes))
			}
		})
	}
}

// TestFailedCustomizeFstab tests failures in the customizeFstab function
func TestFailedCustomizeFstab(t *testing.T) {
	t.Run("test_failed_customize_fstab", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &RootfsType{},
			Customization: &CustomizationType{
				Fstab: []*FstabType{
					{
						Label:        "writable",
						Mountpoint:   "/",
						FSType:       "ext4",
						MountOptions: "defaults",
						Dump:         false,
						FsckOrder:    1,
					},
				},
			},
		}

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err := stateMachine.customizeFstab()
		asserter.AssertErrContains(err, "Error opening fstab")
		osOpenFile = os.OpenFile
	})
}
