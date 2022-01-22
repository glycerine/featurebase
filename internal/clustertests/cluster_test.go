// Copyright 2021 Molecula Corp. All rights reserved.
package clustertest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	pilosa "github.com/molecula/featurebase/v3"
	"github.com/molecula/featurebase/v3/disco"
	picli "github.com/molecula/featurebase/v3/http"
)

func TestClusterStuff(t *testing.T) {
	if os.Getenv("ENABLE_PILOSA_CLUSTER_TESTS") != "1" {
		t.Skip("pilosa cluster tests are not enabled")
	}
	cli1, err := picli.NewInternalClient("pilosa1:10101", picli.GetHTTPClient(nil))
	if err != nil {
		t.Fatalf("getting client: %v", err)
	}
	cli2, err := picli.NewInternalClient("pilosa2:10101", picli.GetHTTPClient(nil))
	if err != nil {
		t.Fatalf("getting client: %v", err)
	}
	cli3, err := picli.NewInternalClient("pilosa3:10101", picli.GetHTTPClient(nil))
	if err != nil {
		t.Fatalf("getting client: %v", err)
	}

	if err := cli1.CreateIndex(context.Background(), "testidx", pilosa.IndexOptions{}); err != nil {
		t.Fatalf("creating index: %v", err)
	}
	if err := cli1.CreateFieldWithOptions(context.Background(), "testidx", "testf", pilosa.FieldOptions{CacheType: pilosa.CacheTypeRanked, CacheSize: 100}); err != nil {
		t.Fatalf("creating field: %v", err)
	}

	req := &pilosa.ImportRequest{
		Index: "testidx",
		Field: "testf",
	}
	req.ColumnIDs = make([]uint64, 10)
	req.RowIDs = make([]uint64, 10)

	for i := 0; i < 1000; i++ {
		req.RowIDs[i%10] = 0
		req.ColumnIDs[i%10] = uint64((i/10)*pilosa.ShardWidth + i%10)
		req.Shard = uint64(i / 10)
		if i%10 == 9 {
			err = cli1.Import(context.Background(), nil, req, &pilosa.ImportOptions{})
			if err != nil {
				t.Fatalf("importing: %v", err)
			}
		}
	}

	// Check query results from each node.
	for i, cli := range []*picli.InternalClient{cli1, cli2, cli3} {
		r, err := cli.Query(context.Background(), "testidx", &pilosa.QueryRequest{Index: "testidx", Query: "Count(Row(testf=0))"})
		if err != nil {
			t.Fatalf("count querying pilosa%d: %v", i, err)
		}
		if r.Results[0].(uint64) != 1000 {
			t.Fatalf("count on pilosa%d after import is %d", i, r.Results[0].(uint64))
		}
	}
	t.Run("long pause", func(t *testing.T) {

		pcmd := exec.Command("/pumba", "pause", "clustertests_pilosa3_1", "--duration", "10s")
		pcmd.Stdout = os.Stdout
		pcmd.Stderr = os.Stderr
		t.Log("pausing pilosa3 for 10s")

		if err := pcmd.Start(); err != nil {
			t.Fatalf("starting pumba command: %v", err)
		}
		if err := pcmd.Wait(); err != nil {
			t.Fatalf("waiting on pumba pause cmd: %v", err)
		}

		t.Log("done with pause, waiting for stability")
		waitForStatus(t, cli1.Status, string(disco.ClusterStateNormal), 30, time.Second)
		t.Log("done waiting for stability")

		// Check query results from each node.
		for i, cli := range []*picli.InternalClient{cli1, cli2, cli3} {
			r, err := cli.Query(context.Background(), "testidx", &pilosa.QueryRequest{Index: "testidx", Query: "Count(Row(testf=0))"})
			if err != nil {
				t.Fatalf("count querying pilosa%d: %v", i, err)
			}
			if r.Results[0].(uint64) != 1000 {
				t.Fatalf("count on pilosa%d after import is %d", i, r.Results[0].(uint64))
			}
		}
	})

	t.Run("backup", func(t *testing.T) {
		// do backup with node 1 down, but restart it after a few seconds
		if err := sendCmd("docker", "stop", "clustertests_pilosa1_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}
		var backupCmd *exec.Cmd
		tmpdir := t.TempDir()
		if backupCmd, err = startCmd(
			"featurebase", "backup", "--host=pilosa1:10101", fmt.Sprintf("--output=%s", tmpdir+"/backuptest")); err != nil {
			t.Fatalf("sending backup command: %v", err)
		}
		time.Sleep(time.Second * 5)
		if err = sendCmd("docker", "start", "clustertests_pilosa1_1"); err != nil {
			t.Fatalf("sending start command: %v", err)
		}

		if err = backupCmd.Wait(); err != nil {
			t.Fatalf("waiting on backup to finish: %v", err)
		}

		client := http.Client{}
		if req, err := http.NewRequest(http.MethodDelete, "http://pilosa1:10101/index/testidx", nil); err != nil {
			t.Fatalf("getting req: %v", err)
		} else if resp, err := client.Do(req); err != nil {
			t.Fatalf("doing request: %v", err)
		} else if resp.StatusCode >= 400 {
			bod, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				t.Logf("reading error body: %v", readErr)
			}
			t.Fatalf("deleting index: code=%d, body=%s", resp.StatusCode, bod)
		}

		var restoreCmd *exec.Cmd
		if restoreCmd, err = startCmd("featurebase", "restore", "-s", tmpdir+"/backuptest", "--host", "pilosa1:10101"); err != nil {
			t.Fatalf("starting restore: %v", err)
		}
		time.Sleep(time.Millisecond * 50)
		if err = sendCmd("docker", "stop", "clustertests_pilosa2_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}

		time.Sleep(time.Second * 10)
		if err = sendCmd("docker", "start", "clustertests_pilosa2_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}
		if err := restoreCmd.Wait(); err != nil {
			t.Fatalf("restore failed: %v", err)
		}

		// now do backup with all nodes down and too short a timeout
		// so it fails. Has be to be all 3 because the cluster has
		// replicas=3 and the backup command will retry on replicas.
		if backupCmd, err = startCmd(
			"featurebase", "backup", "--host=pilosa1:10101", fmt.Sprintf("--output=%s", tmpdir+"/backuptest2"), "--retry-period=200ms"); err != nil {
			t.Fatalf("sending second backup command: %v", err)
		}
		time.Sleep(time.Millisecond * 10) // want the backup to get started, then fail
		if err = sendCmd("docker", "stop", "clustertests_pilosa1_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}
		if err = sendCmd("docker", "stop", "clustertests_pilosa2_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}
		if err = sendCmd("docker", "stop", "clustertests_pilosa3_1"); err != nil {
			t.Fatalf("sending stop command: %v", err)
		}

		time.Sleep(time.Second * 5)

		if err = sendCmd("docker", "start", "clustertests_pilosa1_1"); err != nil {
			t.Fatalf("sending start command: %v", err)
		}
		if err = sendCmd("docker", "start", "clustertests_pilosa2_1"); err != nil {
			t.Fatalf("sending start command: %v", err)
		}
		if err = sendCmd("docker", "start", "clustertests_pilosa3_1"); err != nil {
			t.Fatalf("sending start command: %v", err)
		}
		if err = backupCmd.Wait(); err == nil {
			t.Fatal("backup command should have errored but didn't")
		}

	})
}

func waitForStatus(t *testing.T, stator func(context.Context) (string, error), status string, n int, sleep time.Duration) {
	t.Helper()

	for i := 0; i < n; i++ {
		s, err := stator(context.TODO())
		if err != nil {
			t.Logf("Status (try %d/%d): %v (retrying in %s)", i, n, err, sleep.String())
		} else {
			t.Logf("Status (try %d/%d): curr: %s, expect: %s (retrying in %s)", i, n, s, status, sleep.String())
		}
		if s == status {
			return
		}
		time.Sleep(sleep)
	}

	s, err := stator(context.TODO())
	if err != nil {
		t.Fatalf("querying status: %v", err)
	}
	if status != s {
		waited := time.Duration(n) * sleep
		t.Fatalf("waited %s for status: %s, got: %s", waited.String(), status, s)
	}
}
