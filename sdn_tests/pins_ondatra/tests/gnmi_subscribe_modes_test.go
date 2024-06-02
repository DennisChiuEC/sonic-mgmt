package gnmi_subscribe_modes_test

import (
        "context"
        "errors"
        "fmt"
        "strings"
        "testing"
        "time"

        "github.com/google/go-cmp/cmp"
        "github.com/google/go-cmp/cmp/cmpopts"
        gpb "github.com/openconfig/gnmi/proto/gnmi"
        "github.com/openconfig/ondatra"
        "github.com/openconfig/ondatra/gnmi"
        "github.com/openconfig/ygot/ygot"
        "github.com/sonic-net/sonic-mgmt/sdn_tests/pins_ondatra/infrastructure/binding/pinsbind"
        "github.com/sonic-net/sonic-mgmt/sdn_tests/pins_ondatra/infrastructure/testhelper/testhelper"
        "google.golang.org/grpc"
        "google.golang.org/protobuf/encoding/prototext"
)

const (
        deleteTreePath = "/system/config/"
        deletePath     = "/system/config/hostname"
        timePath       = "/system/state/current-datetime"
        nodePath       = "/system/state/hostname"
        subTreePath    = "/system/state"
        containerPath  = "/components"
        rootPath       = "/"
        onChangePath   = "/interfaces/interface[name=%s]/state/mtu"
        errorResponse  = "expectedError"
        syncResponse   = "expectedSync"

        shortTime  = 5 * time.Second
        mediumTime = 10 * time.Second
        longTime   = 30 * time.Second
)

func TestMain(m *testing.M) {
        ondatra.RunTests(m, pinsbind.New)
}

type subscribeTest struct {
        uuid              string
        reqPath           string
        mode              gpb.SubscriptionList_Mode
        updatesOnly       bool
        subMode           gpb.SubscriptionMode
        sampleInterval    uint64 // nanoseconds
        suppressRedundant bool
        heartbeatInterval uint64 // nanoseconds
        expectError       bool
        timeout           time.Duration
}

type operStatus struct {
        match  bool
        delete bool
        value  string
}

func ignorePaths(path string) bool {
        // Paths that change during root and container level tests
        subPaths := []string{
                // TODO Check back if lb/bond are needed after this bug is corrected.
                "/ethernet/state/counters/",
                "//interfaces/interface[name=Loopback0]",
                "//interfaces/interface[name=bond0]",
                "//qos/interfaces/interface",
                "//snmp/engine/version/",
                "//system/mount-points/mount-point",
                "//system/processes/process",
                "//system/cpus/cpu",
                "//system/crm/threshold",
                "//system/ntp/",
                "//system/memory/",
                "//system/ssh-server/ssh-server-vrfs",
                "/subinterface[index=0]/ipv4/unnumbered/",
                "/subinterface[index=0]/ipv4/sag-ipv4/",
                "/subinterface[index=0]/ipv6/sag-ipv6/",
                "/gnmi-pathz-policy-counters/paths/path",
                "/system/state/boot-time",
                "/system/state/uptime",
        }

        for _, sub := range subPaths {
                if strings.Contains(path, sub) {
                        return true
                }
        }
        return false
}

var skipTest = map[string]bool{
        // TODO Ondatra fails to delete subtree
        "TestGNMISubscribeModes/subscribeDeleteNodeLevel":    true,
        "TestGNMISubscribeModes/subscribeDeleteSubtreeLevel": true,
}

func TestGNMISubscribeModes(t *testing.T) {
        testCases := []struct {
                name     string
                function func(*testing.T)
        }{
                {
                        "subscribeOnChange",
                        subscribeTest{
                                uuid:    "f3c55aed-6522-458d-a3cb-e9eca005bcf1",
                                reqPath: onChangePath,
                                mode:    gpb.SubscriptionList_STREAM,
                                subMode: gpb.SubscriptionMode_ON_CHANGE,
                                timeout: shortTime,
                        }.subModeOnChangeTest,
                },
                {
                        "subscribeOnChangeHeartbeatInterval",
                        subscribeTest{
                                uuid:              "5defcb39-7ffa-4404-8eab-59499b50796e",
                                reqPath:           onChangePath,
                                mode:              gpb.SubscriptionList_STREAM,
                                subMode:           gpb.SubscriptionMode_ON_CHANGE,
                                heartbeatInterval: 2000000000,
                                timeout:           shortTime,
                        }.subModeOnChangeTest,
                },
                {
                        "subscribeOnChangeDefinedNode",
                        subscribeTest{
                                uuid:    "1092dc1a-42c8-4125-b2a0-64596dc340ab",
                                reqPath: onChangePath,
                                mode:    gpb.SubscriptionList_STREAM,
                                subMode: gpb.SubscriptionMode_TARGET_DEFINED,
                                timeout: shortTime,
                        }.subModeOnChangeTest,
                },
                { // TODO UMF returns timeout instead of returning proper error code.
                        "subscribeOnChangeUnsupportedPath",
                        subscribeTest{
                                uuid:           "c003d854-6b41-4b0d-acdf-4cc77bd02252",
                                reqPath:        subTreePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_ON_CHANGE,
                                sampleInterval: 2000000000,
                                expectError:    true,
                                timeout:        shortTime,
                        }.subModeOnChangeTest,
                },
                { // TODO Updates_Only is not filtering properly.
                        "subscribeOnChangeUpdatesOnly",
                        subscribeTest{
                                uuid:        "a242c00e-74e7-4749-83cf-9ee724c64901",
                                reqPath:     onChangePath,
                                mode:        gpb.SubscriptionList_STREAM,
                                subMode:     gpb.SubscriptionMode_ON_CHANGE,
                                updatesOnly: true,
                                timeout:     shortTime,
                        }.subModeOnChangeTest,
                },
                {
                        "subscribeOnceRootLevel",
                        subscribeTest{
                                uuid:    "3507ab19-ffb9-4e30-8958-0bb2dc80b424",
                                reqPath: rootPath,
                                mode:    gpb.SubscriptionList_ONCE,
                                timeout: longTime,
                        }.subModeOnceTest,
                },
                {
                        "subscribeOnceContainerLevel",
                        subscribeTest{
                                uuid:    "42b4af42-2394-4945-b094-2a1130d2002d",
                                reqPath: containerPath,
                                mode:    gpb.SubscriptionList_ONCE,
                                timeout: mediumTime,
                        }.subModeOnceTest,
                },
                {
                        "subscribeOnceSubtreeLevel",
                        subscribeTest{
                                uuid:    "349ef06f-eeaa-45f0-b463-86505dc57131",
                                reqPath: subTreePath,
                                mode:    gpb.SubscriptionList_ONCE,
                                timeout: shortTime,
                        }.subModeOnceTest,
                },
                {
                        "subscribeOnceNodeLevel",
                        subscribeTest{
                                uuid:    "bc3c26cc-259c-4b98-8b96-8f98e084724c",
                                reqPath: nodePath,
                                mode:    gpb.SubscriptionList_ONCE,
                                timeout: shortTime,
                        }.subModeOnceTest,
                },
                {
                        "subscribePollRootLevel",
                        subscribeTest{
                                uuid:    "c658fc60-bd58-4fcc-970c-b994a0cf0e94",
                                reqPath: rootPath,
                                mode:    gpb.SubscriptionList_POLL,
                                timeout: longTime,
                        }.subModePollTest,
                },
                {
                        "subscribePollContainerLevel",
                        subscribeTest{
                                uuid:    "5f424b35-4d7f-44db-a4c2-0bd6f2370301",
                                reqPath: containerPath,
                                mode:    gpb.SubscriptionList_POLL,
                                timeout: mediumTime,
                        }.subModePollTest,
                },
                // TODO Updates_Only is not filtering properly.
                {
                        "subscribeOnceUpdatesOnly",
                        subscribeTest{
                                uuid:        "88b334bd-e835-4cb9-975f-e7b01bd6e1bf",
                                reqPath:     subTreePath,
                                mode:        gpb.SubscriptionList_ONCE,
                                updatesOnly: true,
                                timeout:     shortTime,
                        }.subModeUpdatesTest,
                },
                {
                        "subscribePollUpdatesOnly",
                        subscribeTest{
                                uuid:        "177c8d8d-a51b-448d-96b1-ed3e1dde0629",
                                reqPath:     subTreePath,
                                mode:        gpb.SubscriptionList_POLL,
                                updatesOnly: true,
                                timeout:     shortTime,
                        }.subModeUpdatesTest,
                },
                {
                        "subscribeSampleUpdatesOnly",
                        subscribeTest{
                                uuid:           "ebb593da-4f24-4394-80b9-4463a96843bb",
                                reqPath:        subTreePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                                updatesOnly:    true,
                        }.subModeUpdatesTest,
                },
                {
                        "subscribePollSubtreeLevel",
                        subscribeTest{
                                uuid:    "d298894b-3110-4bb3-b13f-3e572d57791e",
                                reqPath: subTreePath,
                                mode:    gpb.SubscriptionList_POLL,
                                timeout: shortTime,
                        }.subModePollTest,
                },
                {
                        "subscribePollNodeLevel",
                        subscribeTest{
                                uuid:    "cb622b6e-5142-4c59-a6b9-603b45b8bcab",
                                reqPath: nodePath,
                                mode:    gpb.SubscriptionList_POLL,
                                timeout: shortTime,
                        }.subModePollTest,
                },
                {
                        "subscribeSampleSubtreeLevel",
                        subscribeTest{
                                uuid:           "899345da-b715-4caa-a02f-2d03d18c233e",
                                reqPath:        subTreePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeSampleTest,
                },
                { // TODO UMF returns timeout instead of returning proper error code.
                        "subscribeSampleInvalidInterval",
                        subscribeTest{
                                uuid:           "8cf7ea62-5bda-4f71-bd86-fdce88ba2753",
                                reqPath:        nodePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 1,
                                expectError:    true,
                                timeout:        shortTime,
                        }.subModeSampleTest,
                },
                {
                        "subscribeSampleDefinedNode",
                        subscribeTest{
                                uuid:           "45305a9a-c602-421f-8f6a-21d520fea9f8",
                                reqPath:        nodePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_TARGET_DEFINED,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeSampleTest,
                },
                {
                        "subscribeMixedDefinedNode",
                        subscribeTest{
                                uuid:           "ae4c435a-9fa7-494a-94f7-75cd662c3d95",
                                reqPath:        subTreePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_TARGET_DEFINED,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeSampleTest,
                },
                {
                        "subscribeSampleRootLevel",
                        subscribeTest{
                                uuid:           "a495d0b5-482e-411b-9bac-2baa79776293",
                                reqPath:        rootPath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 5000000000,
                                timeout:        longTime,
                        }.subModeRootTest,
                },
                {
                        "subscribeSampleContainerLevel",
                        subscribeTest{
                                uuid:           "aeff11b5-aee2-4689-85e0-a124b5d73506",
                                reqPath:        containerPath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 5000000000,
                                timeout:        longTime,
                        }.subModeSampleTest,
                },
                {
                        "subscribeSampleNodeLevel",
                        subscribeTest{
                                uuid:           "880b3893-da72-44c5-998c-013f0303969f",
                                reqPath:        nodePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeSampleTest,
                },
                {
                        "subscribeDeleteNodeLevel",
                        subscribeTest{
                                uuid:           "529f58c0-8b9b-4820-aeb6-94feb1a68198",
                                reqPath:        deletePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeDeleteTest,
                },
                {
                        "subscribeDeleteSubtreeLevel",
                        subscribeTest{
                                uuid:           "e9d932d5-5fa5-4e00-8c60-cd8823fc34b2",
                                reqPath:        deleteTreePath,
                                mode:           gpb.SubscriptionList_STREAM,
                                subMode:        gpb.SubscriptionMode_SAMPLE,
                                sampleInterval: 2000000000,
                                timeout:        shortTime,
                        }.subModeDeleteTest,
                },
                {
                        "subscribeSampleSuppressRedundant",
                        subscribeTest{
                                uuid:              "5c6e0713-cb8d-43a5-bd8e-8a1ac395eab6",
                                reqPath:           subTreePath,
                                mode:              gpb.SubscriptionList_STREAM,
                                subMode:           gpb.SubscriptionMode_SAMPLE,
                                sampleInterval:    2000000000,
                                suppressRedundant: true,
                                timeout:           shortTime,
                        }.subModeSuppressTest,
                },
                {
                        "subscribeSampleHeartbeat",
                        subscribeTest{
                                uuid:              "8b58ff99-39fc-41e8-9589-b78da0aeca12",
                                reqPath:           subTreePath,
                                mode:              gpb.SubscriptionList_STREAM,
                                subMode:           gpb.SubscriptionMode_SAMPLE,
                                sampleInterval:    2000000000,
                                suppressRedundant: true,
                                heartbeatInterval: 3000000000,
                                timeout:           shortTime,
                        }.subModeSuppressTest,
                },
        }

        dut := ondatra.DUT(t, "DUT")

        // Check if the switch is responsive with Get API, which will panic if the switch does not return
        // state value for specified interface Openconfig path resulting in a test failure.
        intf, err := testhelper.RandomInterface(t, dut, &testhelper.RandomInterfaceParams{OperDownOk: true})
        if err != nil {
                t.Fatalf("Failed to fetch random interface: %v", err)
        }
        gnmi.Get(t, dut, gnmi.OC().Interface(intf).Mtu().State())

        for _, testCase := range testCases {
                t.Run(testCase.name, testCase.function)
        }
}

// Test for gNMI Subscribe Stream mode for OnChange subscriptions.
func (c subscribeTest) subModeOnChangeTest(t *testing.T) {
        defer testhelper.NewTearDownOptions(t).WithID(c.uuid).Teardown(t)
        if skipTest[t.Name()] {
                t.Skip()
        }
        dut := ondatra.DUT(t, "DUT")

        var intf string
        if c.expectError == false {
                var err error
                intf, err = testhelper.RandomInterface(t, dut, nil)
                if err != nil {
                        t.Fatalf("Failed to fetch random interface: %v", err)
                }
                c.reqPath = fmt.Sprintf(c.reqPath, intf)
        }
        subscribeRequest := buildRequest(t, c, dut.Name())
        t.Logf("SubscribeRequest:\n%v", prototext.Format(subscribeRequest))

        ctx, cancel := context.WithCancel(context.Background())
        defer cancel()
        gnmiClient, err := dut.RawAPIs().BindingDUT().DialGNMI(ctx, grpc.WithBlock())
        if err != nil {
                t.Fatalf("Unable to get gNMI client (%v)", err)
        }
        subscribeClient, err := gnmiClient.Subscribe(ctx)
        if err != nil {
                t.Fatalf("Unable to get subscribe client (%v)", err)
        }

        if err := subscribeClient.Send(subscribeRequest); err != nil {
                t.Fatalf("Failed to send gNMI subscribe request (%v)", err)
        }

        expectedPaths := make(map[string]operStatus)
        if !c.updatesOnly {
                expectedPaths = c.buildExpectedPaths(t, dut)
        }
        expectedPaths[syncResponse] = operStatus{}

        foundPaths, _ := collectResponse(t, subscribeClient, expectedPaths)
        if c.expectError {
                foundErr, ok := foundPaths[errorResponse]
                if !ok {
                        t.Fatal("Expected error but got none")
                }
                if !strings.Contains(foundErr.value, "InvalidArgument") {
                        t.Errorf("Error is not an InvalidArgument: %s", foundErr.value)
                }
                return
        }
        if diff := cmp.Diff(expectedPaths, foundPaths, cmpopts.IgnoreUnexported(operStatus{})); diff != "" {
                t.Errorf("collectResponse(expectedPaths):\n%v \nResponse mismatch (-missing +extra):\n%s", expectedPaths, diff)
        }
        delete(expectedPaths, syncResponse)

        got := gnmi.Get(t, dut, gnmi.OC().Interface(intf).Mtu().State())
        defer gnmi.Update(t, dut, gnmi.OC().Interface(intf).Mtu().Config(), got)
        mtu := uint16(1500)
        if got == 1500 {
                mtu = 9000
        }
        if c.heartbeatInterval == 0 {
                gnmi.Update(t, dut, gnmi.OC().Interface(intf).Mtu().Config(), mtu)
        }

        foundPaths, delay := collectResponse(t, subscribeClient, expectedPaths)
        if diff := cmp.Diff(expectedPaths, foundPaths, cmpopts.IgnoreUnexported(operStatus{})); diff != "" {
                t.Errorf("collectResponse(expectedPaths):\n%v \nResponse mismatch (-missing +extra):\n%s", expectedPaths, diff)
        }
        if c.heartbeatInterval != 0 {
                if delay > time.Duration(c.heartbeatInterval+(c.heartbeatInterval/2)) {
                        t.Errorf("Failed sampleInterval with time of %v", delay)
                }
                gnmi.Update(t, dut, gnmi.OC().Interface(intf).Mtu().Config(), mtu)
                foundPaths, _ := collectResponse(t, subscribeClient, expectedPaths)
                if diff := cmp.Diff(expectedPaths, foundPaths, cmpopts.IgnoreUnexported(operStatus{})); diff != "" {
                        t.Errorf("collectResponse(expectedPaths):\n%v \nResponse mismatch (-missing +extra):\n%s", expectedPaths, diff)
                }
        }
}
