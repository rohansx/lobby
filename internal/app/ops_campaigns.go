package app

import (
	"net/http"
)

// Operator endpoints for Launch Pack campaigns and the destination catalogue.
// Registered in campaignRoutes behind cloudOnly and operatorOnly.

func (s *Server) opsDestinations(w http.ResponseWriter, r *http.Request) {
	stage := r.URL.Query().Get("stage")
	if stage != "" && !profileStages[stage] {
		jsonError(w, 400, "Stage must be idea, prelaunch, beta or live.")
		return
	}
	list, err := s.app.Destinations(r.Context(), stage)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

// opsSaveDestination creates (POST) or updates (PATCH {id}) a destination.
// A PATCH decodes over the stored row, so omitted fields keep their values.
func (s *Server) opsSaveDestination(w http.ResponseWriter, r *http.Request) {
	d := &Destination{Mode: "staff", Active: true}
	if r.Method == http.MethodPatch {
		var err error
		if d, err = s.app.DestinationByID(r.Context(), pathInt(r, "id")); err != nil {
			s.packError(w, err)
			return
		}
	}
	id := d.ID
	if err := decodeJSON(r, d); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	d.ID = id
	d, err := s.app.SaveDestination(r.Context(), d)
	if err != nil {
		s.packError(w, err)
		return
	}
	code := 200
	if id == 0 {
		code = 201
	}
	writeJSON(w, code, d)
}

func (s *Server) opsDeleteDestination(w http.ResponseWriter, r *http.Request) {
	if err := s.app.DeleteDestination(r.Context(), pathInt(r, "id")); err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) opsCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Campaigns(r.Context(), 0)
	if err != nil {
		s.packError(w, err)
		return
	}
	states, minutes, err := s.app.taskStats(r.Context())
	if err != nil {
		s.packError(w, err)
		return
	}
	for _, c := range list {
		c.TaskStates, c.TaskMinutes = states[c.ID], minutes[c.ID]
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

// opsCreateCampaign opens a campaign by hand, e.g. for a pack paid outside Polar.
func (s *Server) opsCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID    int64  `json:"account_id"`
		PriceCents   int64  `json:"price_cents"`
		PolarOrderID string `json:"polar_order_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	c, err := s.app.CreateCampaign(r.Context(), body.AccountID, body.PriceCents, body.PolarOrderID)
	if err != nil {
		s.packError(w, err)
		return
	}
	s.app.log.Info("ops: campaign created", "operator", adminID(r), "campaign", c.ID)
	writeJSON(w, 201, c)
}

func (s *Server) opsLoadCampaign(w http.ResponseWriter, r *http.Request) *Campaign {
	c, err := s.app.Campaign(r.Context(), pathInt(r, "id"), 0)
	if err != nil {
		s.packError(w, err)
		return nil
	}
	return c
}

func (s *Server) opsCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.opsLoadCampaign(w, r)
	if c == nil {
		return
	}
	tasks, err := s.app.Tasks(r.Context(), c.ID)
	if err != nil {
		s.packError(w, err)
		return
	}
	out := map[string]any{"campaign": c, "tasks": tasks, "profile": nil, "assets": []Asset{}, "moves": operatorMoves(c.Status)}
	if c.ProjectID != nil {
		if out["profile"], err = s.app.profileView(r.Context(), *c.ProjectID); err == nil {
			out["assets"], err = s.app.Assets(r.Context(), *c.ProjectID)
		}
		if err != nil {
			s.packError(w, err)
			return
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) opsEditCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.opsLoadCampaign(w, r)
	if c == nil {
		return
	}
	var e campaignEdit
	if err := decodeJSON(r, &e); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	c, err := s.app.EditCampaign(r.Context(), c, e)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (s *Server) opsMove(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := s.opsLoadCampaign(w, r)
		if c == nil {
			return
		}
		c, err := s.app.moveCampaign(r.Context(), c, to, byOperator, "")
		if err != nil {
			s.packError(w, err)
			return
		}
		writeJSON(w, 200, c)
	}
}

func (s *Server) opsAddTask(w http.ResponseWriter, r *http.Request) {
	c := s.opsLoadCampaign(w, r)
	if c == nil {
		return
	}
	var e taskEdit
	if err := decodeJSON(r, &e); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	t, err := s.app.AddTask(r.Context(), c, e)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) opsEditTask(w http.ResponseWriter, r *http.Request) {
	c := s.opsLoadCampaign(w, r)
	if c == nil {
		return
	}
	var e taskEdit
	if err := decodeJSON(r, &e); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	t, err := s.app.EditTask(r.Context(), c, pathInt(r, "tid"), e)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) opsDeleteTask(w http.ResponseWriter, r *http.Request) {
	c := s.opsLoadCampaign(w, r)
	if c == nil {
		return
	}
	if err := s.app.DeleteTask(r.Context(), c, pathInt(r, "tid")); err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// operatorMoves lists the statuses an operator can move a campaign to from status, per checkMove.
func operatorMoves(status string) []string {
	out := []string{}
	for _, to := range []string{statusIntake, statusReadiness, statusPlanning, statusAwaitingApproval, statusExecuting,
		statusReporting, statusDone, statusCancelled} {
		if checkMove(status, to, byOperator) == nil {
			out = append(out, to)
		}
	}
	return out
}
