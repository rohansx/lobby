package app

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
)

// Launch Pack endpoints for founders, all cloud-only and scoped to the
// signed-in account: another account's campaign, profile or files read as 404.

func (s *Server) campaignRoutes(mux *http.ServeMux) {
	f := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.cloudOnly(h)) }
	op := func(pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, s.cloudOnly(s.operatorOnly(h))) }
	const c, o = "/api/admin/campaigns/{id}", "/api/admin/ops/campaigns/{id}"

	f("GET /api/admin/campaigns", s.adminCampaigns)
	f("GET "+c, s.adminCampaign)
	f("PUT "+c+"/project", s.adminCampaignProject)
	f("POST "+c+"/submit-intake", s.adminCampaignMove(statusReadiness))
	f("POST "+c+"/approve", s.adminCampaignMove(statusExecuting))
	f("POST "+c+"/request-changes", s.adminCampaignMove(statusPlanning))
	f("PATCH "+c+"/tasks/{tid}", s.adminCampaignTask)
	f("GET "+c+"/report", s.adminCampaignReport)
	f("PUT /api/admin/projects/{slug}/signups/{id}/relevant", s.adminSignupRelevant)
	f("GET /api/admin/projects/{slug}/profile", s.adminProfile)
	f("POST /api/admin/projects/{slug}/profile", s.adminSaveProfile)
	f("GET /api/admin/projects/{slug}/assets", s.adminAssets)
	f("POST /api/admin/projects/{slug}/assets", s.adminUploadAsset)
	f("GET /api/admin/projects/{slug}/assets/{id}", s.adminAsset)
	f("DELETE /api/admin/projects/{slug}/assets/{id}", s.adminDeleteAsset)

	op("GET /api/admin/ops/destinations", s.opsDestinations)
	op("POST /api/admin/ops/destinations", s.opsSaveDestination)
	op("PATCH /api/admin/ops/destinations/{id}", s.opsSaveDestination)
	op("DELETE /api/admin/ops/destinations/{id}", s.opsDeleteDestination)
	op("GET /api/admin/ops/campaigns", s.opsCampaigns)
	op("POST /api/admin/ops/campaigns", s.opsCreateCampaign)
	op("GET "+o, s.opsCampaign)
	op("PATCH "+o, s.opsEditCampaign)
	op("POST "+o+"/tasks", s.opsAddTask)
	op("PATCH "+o+"/tasks/{tid}", s.opsEditTask)
	op("DELETE "+o+"/tasks/{tid}", s.opsDeleteTask)
	op("POST "+o+"/send-for-approval", s.opsMove(statusAwaitingApproval))
	op("POST "+o+"/publish-report", s.opsMove(statusDone))
	op("GET "+o+"/report", s.opsCampaignReport)
}

func pathInt(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

// packError answers with the status that fits err: 404, 409, 400 or 500.
func (s *Server) packError(w http.ResponseWriter, err error) {
	var ce *ErrConflict
	var ve *ValidationError
	switch {
	case errors.Is(err, ErrNotFound):
		jsonError(w, 404, "not found")
	case errors.As(err, &ce):
		jsonError(w, 409, ce.Msg)
	case errors.As(err, &ve):
		jsonError(w, 400, ve.Msg)
	default:
		s.app.log.Error("launch pack", "err", err)
		jsonError(w, 500, "Something went wrong. Try again in a minute.")
	}
}

// founderCampaign loads the {id} campaign of the signed-in account.
func (s *Server) founderCampaign(w http.ResponseWriter, r *http.Request) *Campaign {
	c, err := s.app.Campaign(r.Context(), pathInt(r, "id"), adminID(r))
	if err != nil {
		s.packError(w, err)
		return nil
	}
	return c
}

func (s *Server) adminCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Campaigns(r.Context(), adminID(r))
	if err != nil {
		s.packError(w, err)
		return
	}
	for i, c := range list {
		list[i] = c.forFounder()
	}
	writeJSON(w, 200, map[string]any{"data": list})
}

func (s *Server) adminCampaign(w http.ResponseWriter, r *http.Request) {
	c := s.founderCampaign(w, r)
	if c == nil {
		return
	}
	tasks, err := s.app.Tasks(r.Context(), c.ID)
	if err != nil {
		s.packError(w, err)
		return
	}
	for _, t := range tasks {
		t.Minutes = 0
	}
	writeJSON(w, 200, map[string]any{"campaign": c.forFounder(), "tasks": tasks})
}

func (s *Server) adminCampaignProject(w http.ResponseWriter, r *http.Request) {
	c := s.founderCampaign(w, r)
	if c == nil {
		return
	}
	var body struct{ Slug string }
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	p, err := s.app.ProjectBySlug(r.Context(), body.Slug)
	if err == nil {
		err = s.app.SetCampaignProject(r.Context(), c, p)
	}
	if err != nil {
		s.packError(w, err)
		return
	}
	c, _ = s.app.Campaign(r.Context(), c.ID, adminID(r))
	writeJSON(w, 200, c.forFounder())
}

func (s *Server) adminCampaignMove(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := s.founderCampaign(w, r)
		if c == nil {
			return
		}
		var body struct{ Note string }
		if err := decodeJSON(r, &body); err != nil {
			jsonError(w, 400, "invalid JSON body")
			return
		}
		c, err := s.app.moveCampaign(r.Context(), c, to, byFounder, body.Note)
		if err != nil {
			s.packError(w, err)
			return
		}
		writeJSON(w, 200, c.forFounder())
	}
}

func (s *Server) adminCampaignTask(w http.ResponseWriter, r *http.Request) {
	c := s.founderCampaign(w, r)
	if c == nil {
		return
	}
	var e founderTaskEdit
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&e); err != nil {
		jsonError(w, 400, "You can set state (submitted or live), listing_url and evidence.")
		return
	}
	t, err := s.app.FounderEditTask(r.Context(), c, pathInt(r, "tid"), e)
	if err != nil {
		s.packError(w, err)
		return
	}
	t.Minutes = 0
	writeJSON(w, 200, t)
}

// packProject loads the {slug} project for its owner, or for an operator when read is set.
func (s *Server) packProject(w http.ResponseWriter, r *http.Request, read bool) *Project {
	p, err := s.app.ProjectBySlug(r.Context(), r.PathValue("slug"))
	if err == nil && p.OwnerID != adminID(r) {
		if ac, aerr := s.app.Account(r.Context(), adminID(r)); !read || aerr != nil || !s.app.isOperator(ac) {
			err = ErrNotFound
		}
	}
	if err != nil {
		jsonError(w, 404, "project not found")
		return nil
	}
	return p
}

func (s *Server) adminProfile(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, true)
	if p == nil {
		return
	}
	out, err := s.app.profileView(r.Context(), p.ID)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) adminSaveProfile(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, false)
	if p == nil {
		return
	}
	var prof ProductProfile
	if err := decodeJSON(r, &prof); err != nil {
		jsonError(w, 400, "invalid JSON body")
		return
	}
	pv, err := s.app.SaveProfile(r.Context(), p.ID, adminID(r), prof)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"version": pv, "missing": pv.Profile.Missing()})
}

func (s *Server) adminAssets(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, true)
	if p == nil {
		return
	}
	list, err := s.app.Assets(r.Context(), p.ID)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": list, "max": maxAssetsPerProject, "max_bytes": maxAssetBytes})
}

func (s *Server) adminUploadAsset(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, false)
	if p == nil {
		return
	}
	// Files are stored in the database, so only Launch Pack buyers get to add them.
	if ok, err := s.app.hasCampaign(r.Context(), p.OwnerID); err != nil {
		s.packError(w, err)
		return
	} else if !ok {
		jsonError(w, 403, "Uploads open once you have a Launch Pack.")
		return
	}
	// Room for the multipart framing and the kind field around a 5 MB file.
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetBytes+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			jsonError(w, 413, "Files can be up to 5 MB.")
			return
		}
		jsonError(w, 400, "Send the file as multipart form data.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, hdr, err := r.FormFile("file")
	if err != nil {
		jsonError(w, 400, "Choose a file to upload.")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAssetBytes+1))
	if err != nil {
		jsonError(w, 400, "Couldn't read the file.")
		return
	}
	if len(data) > maxAssetBytes {
		jsonError(w, 413, "Files can be up to 5 MB.")
		return
	}
	as, err := s.app.AddAsset(r.Context(), p.ID, r.FormValue("kind"), hdr.Filename, data)
	if err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 201, as)
}

func (s *Server) adminAsset(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, true)
	if p == nil {
		return
	}
	as, data, err := s.app.AssetData(r.Context(), p.ID, pathInt(r, "id"))
	if err != nil {
		s.packError(w, err)
		return
	}
	disp := "inline"
	if as.Mime == "application/pdf" {
		disp = "attachment"
	}
	h := w.Header()
	h.Set("Content-Type", as.Mime)
	h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": as.Filename}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; sandbox")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func (s *Server) adminDeleteAsset(w http.ResponseWriter, r *http.Request) {
	p := s.packProject(w, r, false)
	if p == nil {
		return
	}
	if err := s.app.DeleteAsset(r.Context(), p.ID, pathInt(r, "id")); err != nil {
		s.packError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
