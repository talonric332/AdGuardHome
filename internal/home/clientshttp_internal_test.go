package home

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AdguardTeam/AdGuardHome/internal/client"
	"github.com/AdguardTeam/AdGuardHome/internal/filtering"
	"github.com/AdguardTeam/AdGuardHome/internal/schedule"
	"github.com/stretchr/testify/require"
)

const (
	testClientIP1 = "1.1.1.1"
	testClientIP2 = "2.2.2.2"
)

// newPersistentClient is a helper function that returns a persistent client
// with the specified name and newly generated UID.
func newPersistentClient(name string) (c *client.Persistent) {
	return &client.Persistent{
		Name: name,
		UID:  client.MustNewUID(),
		BlockedServices: &filtering.BlockedServices{
			Schedule: &schedule.Weekly{},
		},
	}
}

// newPersistentClientWithIDs is a helper function that returns a persistent
// client with the specified name and ids.
func newPersistentClientWithIDs(tb testing.TB, name string, ids []string) (c *client.Persistent) {
	tb.Helper()

	c = newPersistentClient(name)
	err := c.SetIDs(ids)
	require.NoError(tb, err)

	return c
}

// clientsCompare is a helper function that uses HTTP API to check whether want
// persistent clients are the same as the persistent clients stored in the
// clients container.
func clientsCompare(tb testing.TB, clients *clientsContainer, want []*client.Persistent) (ok bool) {
	tb.Helper()

	rw := httptest.NewRecorder()
	clients.handleGetClients(rw, &http.Request{})

	body, err := io.ReadAll(rw.Body)
	require.NoError(tb, err)

	clientList := &clientListJSON{}
	err = json.Unmarshal(body, clientList)
	require.NoError(tb, err)

	got := map[string]*client.Persistent{}
	for _, cj := range clientList.Clients {
		var c *client.Persistent
		c, err = clients.jsonToClient(*cj, nil)
		require.NoError(tb, err)

		got[c.Name] = c
	}
	require.Len(tb, want, len(got))

	for _, c := range want {
		var gotClient *client.Persistent
		gotClient, ok = got[c.Name]
		if !ok || !gotClient.EqualIDs(c) {
			return false
		}
	}

	return true
}

func TestClientsContainer_HandleAddClient(t *testing.T) {
	clients := newClientsContainer(t)

	clientOne := newPersistentClientWithIDs(t, "client1", []string{testClientIP1})
	clientTwo := newPersistentClientWithIDs(t, "client2", []string{testClientIP2})

	clientEmptyID := newPersistentClient("empty_client_id")
	clientEmptyID.ClientIDs = []string{""}

	testCases := []struct {
		name       string
		client     *client.Persistent
		wantCode   int
		wantClient []*client.Persistent
	}{{
		name:       "add_one",
		client:     clientOne,
		wantCode:   http.StatusOK,
		wantClient: []*client.Persistent{clientOne},
	}, {
		name:       "add_two",
		client:     clientTwo,
		wantCode:   http.StatusOK,
		wantClient: []*client.Persistent{clientOne, clientTwo},
	}, {
		name:       "duplicate_client",
		client:     clientTwo,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientOne, clientTwo},
	}, {
		name:       "empty_client_id",
		client:     clientEmptyID,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientOne, clientTwo},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cj := clientToJSON(tc.client)

			body, err := json.Marshal(cj)
			require.NoError(t, err)

			rw := httptest.NewRecorder()
			r, err := http.NewRequest(http.MethodPost, "", bytes.NewReader(body))
			require.NoError(t, err)

			clients.handleAddClient(rw, r)
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, rw.Code)

			ok := clientsCompare(t, clients, tc.wantClient)
			require.True(t, ok)
		})
	}
}

func TestClientsContainer_HandleDelClient(t *testing.T) {
	clients := newClientsContainer(t)

	clientOne := newPersistentClientWithIDs(t, "client1", []string{testClientIP1})
	err := clients.add(clientOne)
	require.NoError(t, err)

	clientTwo := newPersistentClientWithIDs(t, "client2", []string{testClientIP2})
	err = clients.add(clientTwo)
	require.NoError(t, err)

	ok := clientsCompare(t, clients, []*client.Persistent{clientOne, clientTwo})
	require.True(t, ok)

	testCases := []struct {
		name       string
		client     *client.Persistent
		wantCode   int
		wantClient []*client.Persistent
	}{{
		name:       "remove_one",
		client:     clientOne,
		wantCode:   http.StatusOK,
		wantClient: []*client.Persistent{clientTwo},
	}, {
		name:       "duplicate_client",
		client:     clientOne,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientTwo},
	}, {
		name:       "empty_client_name",
		client:     newPersistentClient(""),
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientTwo},
	}, {
		name:       "remove_two",
		client:     clientTwo,
		wantCode:   http.StatusOK,
		wantClient: []*client.Persistent{},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cj := clientToJSON(tc.client)

			var body []byte
			body, err = json.Marshal(cj)
			require.NoError(t, err)

			rw := httptest.NewRecorder()
			var r *http.Request
			r, err = http.NewRequest(http.MethodPost, "", bytes.NewReader(body))
			require.NoError(t, err)

			clients.handleDelClient(rw, r)
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, rw.Code)

			ok = clientsCompare(t, clients, tc.wantClient)
			require.True(t, ok)
		})
	}
}

func TestClientsContainer_HandleUpdateClient(t *testing.T) {
	clients := newClientsContainer(t)

	clientOne := newPersistentClientWithIDs(t, "client1", []string{testClientIP1})
	err := clients.add(clientOne)
	require.NoError(t, err)

	ok := clientsCompare(t, clients, []*client.Persistent{clientOne})
	require.True(t, ok)

	clientModified := newPersistentClientWithIDs(t, "client2", []string{testClientIP2})

	clientEmptyID := newPersistentClient("empty_client_id")
	clientEmptyID.ClientIDs = []string{""}

	testCases := []struct {
		name       string
		clientName string
		modified   *client.Persistent
		wantCode   int
		wantClient []*client.Persistent
	}{{
		name:       "update_one",
		clientName: clientOne.Name,
		modified:   clientModified,
		wantCode:   http.StatusOK,
		wantClient: []*client.Persistent{clientModified},
	}, {
		name:       "empty_name",
		clientName: "",
		modified:   clientOne,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientModified},
	}, {
		name:       "client_not_found",
		clientName: "client_not_found",
		modified:   clientOne,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientModified},
	}, {
		name:       "empty_client_id",
		clientName: clientModified.Name,
		modified:   clientEmptyID,
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientModified},
	}, {
		name:       "no_ids",
		clientName: clientModified.Name,
		modified:   newPersistentClient("no_ids"),
		wantCode:   http.StatusBadRequest,
		wantClient: []*client.Persistent{clientModified},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uj := updateJSON{
				Name: tc.clientName,
				Data: *clientToJSON(tc.modified),
			}

			var body []byte
			body, err = json.Marshal(uj)
			require.NoError(t, err)

			rw := httptest.NewRecorder()
			var r *http.Request
			r, err = http.NewRequest(http.MethodPost, "", bytes.NewReader(body))
			require.NoError(t, err)

			clients.handleUpdateClient(rw, r)
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, rw.Code)

			ok = clientsCompare(t, clients, tc.wantClient)
			require.True(t, ok)
		})
	}
}
