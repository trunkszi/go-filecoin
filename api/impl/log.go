package impl

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	logging "gx/ipfs/QmRREK2CAZ5Re2Bd9zZFG6FeYDppUWt5cMgsoUEp3ktgSr/go-log"
	writer "gx/ipfs/QmRREK2CAZ5Re2Bd9zZFG6FeYDppUWt5cMgsoUEp3ktgSr/go-log/writer"
	manet "gx/ipfs/QmV6FjemM1K8oXjrvuq3wuVWWoU2TLDPmNnKrxHzY3v6Ai/go-multiaddr-net"
	ma "gx/ipfs/QmYmsdtJ3HsodkePE3eU3TsCaP2YvPZJ4LoXnNkDE5Tpt7/go-multiaddr"
)

var log = logging.Logger("api/impl")

type nodeLog struct {
	api *nodeAPI
}

func newNodeLog(api *nodeAPI) *nodeLog {
	return &nodeLog{api: api}
}

func (api *nodeLog) Tail(ctx context.Context) io.Reader {
	r, w := io.Pipe()
	go func() {
		defer w.Close() // nolint: errcheck
		<-ctx.Done()
	}()

	writer.WriterGroup.AddWriter(w)

	return r
}

func (api *nodeLog) StreamTo(ctx context.Context, maddr ma.Multiaddr) error {
	nodeDetails, err := api.api.ID().Details()
	if err != nil {
		return err
	}
	peerID := nodeDetails.ID
	// Get the nodes nickname.
	nodeNic, err := api.api.Config().Get("stats.nickname")
	if err != nil {
		return err
	}

	// connection the logs will stream to
	mconn, err := manet.Dial(maddr)
	if err != nil {
		return err
	}
	defer mconn.Close() // nolint: errcheck
	wconn := bufio.NewWriter(mconn)

	r, w := io.Pipe()
	go func() {
		defer w.Close() // nolint: errcheck
		defer r.Close() // nolint: errcheck
		<-ctx.Done()
	}()

	// add the pipe to the event log writer group
	writer.WriterGroup.AddWriter(w)

	/*** THIS IS A HACK FOR DEMO ***/
	// Lets make a crappy filter
	filterR, filterW := io.Pipe()
	go func() {
		defer filterR.Close() // nolint: errcheck
		defer filterW.Close() // nolint: errcheck
		<-ctx.Done()
	}()

	// We need this filter to ensure every log message has the peerID and nickname
	filterDecoder := json.NewDecoder(r)
	filterEncoder := json.NewEncoder(filterW)
	go func() {
		for {
			if ctx.Err() != nil {
				log.Warningf("filter context error, closing: %v", ctx.Err())
				break
			}
			var event map[string]interface{}
			if err := filterDecoder.Decode(&event); err != nil {
				log.Warningf("failed to decode event: %v", err)
				continue
			}
			// "filter"
			// add things to the event log here
			event["peerName"] = nodeNic
			event["peerID"] = peerID
			if err := filterEncoder.Encode(event); err != nil {
				log.Warningf("failed to encode event: %v", err)
				continue
			}
		}
	}()

	_, err = wconn.ReadFrom(filterR)
	if err != nil {
		return err
	}
	// flush the rest of the events that may be in the pipe before the defered close
	wconn.Flush() // nolint: errcheck

	return nil
}
