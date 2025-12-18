package handlers

import (
	"encoding/json"
	"net/http"

	"modem-manager/models"
	"modem-manager/services"
)

var serialManager = services.GetSerialManager()

// ListModems returns a list of available modems
func ListModems(w http.ResponseWriter, r *http.Request) {
	if ports, err := serialManager.Scan(115200); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
	} else {
		respondJSON(w, http.StatusOK, ports)
	}
}

// SendATCommand sends a raw AT command to the modem
func SendATCommand(w http.ResponseWriter, r *http.Request) {
	var cmd models.ATCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if svc := getService(w, cmd.Port); svc != nil {
		var err error
		if cmd.Response, err = svc.SendATCommand(cmd.Command); err != nil {
			cmd.Error = err.Error()
		}
		respondJSON(w, http.StatusOK, cmd)
	}
}

// GetModemInfo retrieves detailed information about the modem
func GetModemInfo(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if info, err := svc.GetModemInfo(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, info)
		}
	}
}

// GetSignalStrength retrieves the current signal strength
func GetSignalStrength(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if signal, err := svc.GetSignalStrength(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, signal)
		}
	}
}

// ListSMS retrieves all SMS messages from the modem
func ListSMS(w http.ResponseWriter, r *http.Request) {
	if svc := getService(w, r.URL.Query().Get("port")); svc != nil {
		if list, err := svc.ListSMS(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, list)
		}
	}
}

// SendSMS sends an SMS message
func SendSMS(w http.ResponseWriter, r *http.Request) {
	var req models.SendSMSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if svc := getService(w, req.Port); svc != nil {
		if err := svc.SendSMS(req.Number, req.Message); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
		} else {
			respondJSON(w, http.StatusOK, map[string]string{"status": "sent"})
		}
	}
}

// Helper functions

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

func getService(w http.ResponseWriter, port string) *services.SerialService {
	if port == "" {
		respondError(w, http.StatusBadRequest, "port is required")
		return nil
	}
	svc, err := serialManager.GetService(port)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return nil
	}
	return svc
}
