package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/internal/api"
	"github.com/burnerpad/burnerpad-cli/internal/id"
	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/internal/term"
)

type createReceipt struct {
	Status    *string `json:"status"`
	Server    *string `json:"server"`
	Link      *string `json:"link"`
	MgmtToken *string `json:"mgmt_token"`
}

func runBurn(a *application, flags *burnFlags, positionals []string) error {
	if len(positionals) > 1 {
		return usage("invalid_input", "burn accepts one share URL or identifier")
	}
	if countSources(flags.tokenFile != "", flags.tokenFD != -1) > 1 {
		return usage("invalid_credential_source", "select exactly one management-token source")
	}
	if flags.tokenFile == "-" || (flags.tokenFD != -1 && flags.tokenFD < 3) {
		return usage("invalid_credential_source", "token files must be named and token descriptors must be 3 or greater")
	}
	if len(positionals) == 1 && a.env.StdinPiped {
		return usage("invalid_input", "a burn target cannot be mixed with a piped create receipt")
	}
	var target id.Target
	var server, token string
	var err error
	if len(positionals) == 0 && a.env.StdinPiped {
		if flags.tokenFile != "" || flags.tokenFD != -1 {
			return usage("invalid_credential_source", "a create receipt already contains its management token")
		}
		data, readErr := readBounded(a.env.Stdin, 16*1024)
		if readErr != nil {
			return readErr
		}
		var receipt createReceipt
		decoder := json.NewDecoder(bytes.NewReader(data))
		if decoder.Decode(&receipt) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
			receipt.Status == nil || *receipt.Status != "created" || receipt.Server == nil || receipt.Link == nil || receipt.MgmtToken == nil {
			return usage("invalid_input", "stdin is not a valid Burnerpad create receipt")
		}
		target, err = id.ParseShareURL(*receipt.Link)
		if err != nil {
			return usage("invalid_input", "the create receipt is internally inconsistent")
		}
		server, err = api.ParseBaseURL(*receipt.Server)
		if err != nil || server != target.Origin {
			return usage("invalid_input", "the create receipt is internally inconsistent")
		}
		token = *receipt.MgmtToken
		if a.cfg.serverFlag {
			if err := a.warnBeforeNetwork("--server is ignored for a piped create receipt"); err != nil {
				return attachServer(err, server)
			}
		}
	} else {
		if len(positionals) != 1 {
			return usage("invalid_input", "burn needs a create receipt, full share URL, or identifier")
		}
		target, err = id.ParseBurnTarget(positionals[0])
		if err != nil {
			return usage("invalid_input", "burn needs a full share URL or current 26-character identifier")
		}
		if target.Origin != "" {
			server, err = api.ParseBaseURL(target.Origin)
			if err != nil {
				return usage("invalid_input", "the share URL does not name a safe server origin")
			}
			if a.cfg.serverFlag {
				if err := a.warnBeforeNetwork("--server is ignored because the share URL selects the server"); err != nil {
					return attachServer(err, server)
				}
			}
		} else {
			server, err = api.ParseBaseURL(a.cfg.server)
			if err != nil {
				return usage("invalid_input", "the selected server is not a safe origin")
			}
		}
		token, err = a.readToken(flags)
		if err != nil {
			return attachServer(err, server)
		}
	}
	if !validMgmtToken(token) {
		return attachServer(usage("invalid_credential_source", "the management token has an invalid format"), server)
	}
	client, err := api.New(api.Config{BaseURL: server, Timeout: a.cfg.timeout, UserAgent: "burnerpad-cli/" + a.env.Version})
	if err != nil {
		return attachServer(usage("invalid_input", "the selected server is not a safe origin"), server)
	}
	if err := a.discloseServer(server); err != nil {
		return err
	}
	if err := client.Burn(context.Background(), target.ID, token); err != nil {
		return mapAPIError(err, server)
	}
	if a.cfg.json {
		if err := writeJSON(a.env.Stdout, burnResult{Status: "burned", Server: server}); err != nil {
			return attachServer(local("cannot write JSON output"), server)
		}
	} else {
		if _, err := fmt.Fprintln(a.env.Stdout, "burned"); err != nil {
			return attachServer(local("cannot write burn confirmation"), server)
		}
	}
	return nil
}

func (a *application) readToken(flags *burnFlags) (string, error) {
	var raw []byte
	switch {
	case flags.tokenFile != "":
		f, err := os.Open(flags.tokenFile)
		if err != nil {
			return "", local("cannot read the management-token file")
		}
		raw, err = readBounded(f, 256)
		f.Close()
		if err != nil {
			return "", err
		}
	case flags.tokenFD != -1:
		f := os.NewFile(uintptr(flags.tokenFD), "token-fd")
		if f == nil {
			return "", local("cannot read the management-token descriptor")
		}
		var err error
		raw, err = readBounded(f, 256)
		f.Close()
		if err != nil {
			return "", err
		}
	default:
		t, err := a.terminal()
		if err != nil {
			return "", usage("invalid_credential_source", "a protected management-token source is required")
		}
		raw, err = term.ReadPassword(t, "Management token: ")
		if err != nil {
			if errors.Is(err, term.ErrInterrupted) {
				return "", commandError{exit: 130}
			}
			return "", local("cannot read the management token from the terminal")
		}
	}
	raw = trimOneNewline(raw)
	token := string(raw)
	secret.Wipe(raw)
	return token, nil
}

func validMgmtToken(token string) bool {
	decoded, err := envelope.DecodeCanonical([]byte(token))
	if err != nil {
		return false
	}
	defer secret.Wipe(decoded)
	return len(decoded) == 32
}
