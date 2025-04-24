package main

import (
	"errors"
	"log"

	"github.com/msteinert/pam"
)

func authenticateWithPAM(username, password string) bool {
	t, err := pam.StartFunc("webui", username, func(style pam.Style, msg string) (string, error) {
		switch style {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return "", nil
		case pam.ErrorMsg, pam.TextInfo:
			log.Println(msg)
			return "", nil
		default:
			return "", errors.New("unrecognized message style")
		}
	})

	if err != nil {
		log.Printf("PAM start error: %v", err)
		return false
	}

	// Authenticate the user
	err = t.Authenticate(0)
	if err != nil {
		log.Printf("PAM authentication error: %v", err)
		return false
	}

	// Check account validity
	err = t.AcctMgmt(0)
	if err != nil {
		log.Printf("PAM account error: %v", err)
		return false
	}

	return true
}
