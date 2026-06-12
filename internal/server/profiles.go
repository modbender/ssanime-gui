package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/modbender/ssanime-gui/internal/encode"
	"github.com/modbender/ssanime-gui/internal/store"
)

func (h *Handler) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.store.Read().ListEncodeProfiles(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list profiles")
		return
	}
	out := make([]ProfileResponse, len(profiles))
	for i, p := range profiles {
		out[i] = toProfileResponse(p)
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req CreateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		WriteError(w, http.StatusBadRequest, "name required")
		return
	}

	var outRes *string
	if len(req.OutputResolutions) > 0 {
		b, _ := json.Marshal(req.OutputResolutions)
		s := string(b)
		outRes = &s
	}
	var smartblur, deinterlace *int64
	if req.Smartblur != nil {
		v := boolToInt64(*req.Smartblur)
		smartblur = &v
	}
	if req.Deinterlace != nil {
		v := boolToInt64(*req.Deinterlace)
		deinterlace = &v
	}

	profile, err := h.store.Write().CreateEncodeProfile(r.Context(), store.CreateEncodeProfileParams{
		Uuid:              mustUUID(),
		Name:              req.Name,
		Builtin:           0,
		ParentID:          req.ParentID,
		Codec:             req.Codec,
		Crf:               req.CRF,
		Preset:            req.Preset,
		Smartblur:         smartblur,
		Deinterlace:       deinterlace,
		Deblock:           req.Deblock,
		PsyRd:             req.PsyRD,
		PsyRdoq:           req.PsyRDOQ,
		AqStrength:        req.AQStrength,
		AqMode:            req.AQMode,
		Scale:             req.Scale,
		Audio:             req.Audio,
		Container:         req.Container,
		X265Params:        req.X265Params,
		OutputResolutions: outRes,
	})
	if err != nil {
		h.logger.Error("create profile", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}
	WriteJSON(w, http.StatusCreated, toProfileResponse(profile))
}

func (h *Handler) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	existing, err := h.store.Read().GetEncodeProfile(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}
	if existing.Builtin == 1 {
		WriteError(w, http.StatusForbidden, "builtin profiles are immutable")
		return
	}

	var req PatchProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Merge patch: start from existing, override non-nil fields.
	p := store.UpdateEncodeProfileParams{
		ID:                id,
		Name:              existing.Name,
		ParentID:          existing.ParentID,
		Codec:             existing.Codec,
		Crf:               existing.Crf,
		Preset:            existing.Preset,
		Smartblur:         existing.Smartblur,
		Deinterlace:       existing.Deinterlace,
		Deblock:           existing.Deblock,
		PsyRd:             existing.PsyRd,
		PsyRdoq:           existing.PsyRdoq,
		AqStrength:        existing.AqStrength,
		AqMode:            existing.AqMode,
		Scale:             existing.Scale,
		Audio:             existing.Audio,
		Container:         existing.Container,
		X265Params:        existing.X265Params,
		OutputResolutions: existing.OutputResolutions,
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.ParentID != nil {
		p.ParentID = req.ParentID
	}
	if req.Codec != nil {
		p.Codec = req.Codec
	}
	if req.CRF != nil {
		p.Crf = req.CRF
	}
	if req.Preset != nil {
		p.Preset = req.Preset
	}
	if req.Smartblur != nil {
		v := boolToInt64(*req.Smartblur)
		p.Smartblur = &v
	}
	if req.Deinterlace != nil {
		v := boolToInt64(*req.Deinterlace)
		p.Deinterlace = &v
	}
	if req.Deblock != nil {
		p.Deblock = req.Deblock
	}
	if req.PsyRD != nil {
		p.PsyRd = req.PsyRD
	}
	if req.PsyRDOQ != nil {
		p.PsyRdoq = req.PsyRDOQ
	}
	if req.AQStrength != nil {
		p.AqStrength = req.AQStrength
	}
	if req.AQMode != nil {
		p.AqMode = req.AQMode
	}
	if req.Scale != nil {
		p.Scale = req.Scale
	}
	if req.Audio != nil {
		p.Audio = req.Audio
	}
	if req.Container != nil {
		p.Container = req.Container
	}
	if req.X265Params != nil {
		p.X265Params = req.X265Params
	}
	if len(req.OutputResolutions) > 0 {
		b, _ := json.Marshal(req.OutputResolutions)
		s := string(b)
		p.OutputResolutions = &s
	}

	profile, err := h.store.Write().UpdateEncodeProfile(ctx, p)
	if err != nil {
		h.logger.Error("update profile", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	WriteJSON(w, http.StatusOK, toProfileResponse(profile))
}

func (h *Handler) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	existing, err := h.store.Read().GetEncodeProfile(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}
	if existing.Builtin == 1 {
		WriteError(w, http.StatusForbidden, "builtin profiles cannot be deleted")
		return
	}
	if err := h.store.Write().DeleteEncodeProfile(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete profile")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) handleGetResolvedProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	resolver := encode.NewProfileResolver(h.store)
	res, err := resolver.Resolve(r.Context(), id)
	if err != nil {
		h.logger.Error("resolve profile", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to resolve profile")
		return
	}
	WriteJSON(w, http.StatusOK, ResolvedProfileResponse{
		ProfileID:         res.ProfileID,
		Codec:             res.Codec,
		CRF:               res.CRF,
		Preset:            res.Preset,
		SmartBlur:         res.SmartBlur,
		Deinterlace:       res.Deinterlace,
		Deblock:           res.Deblock,
		PsyRD:             res.PsyRD,
		PsyRDOQ:           res.PsyRDOQ,
		AQStrength:        res.AQStrength,
		AQMode:            res.AQMode,
		Audio:             res.Audio,
		Container:         res.Container,
		X265Params:        res.X265Params,
		OutputResolutions: res.OutputResolutions,
	})
}
