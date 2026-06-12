package server

import (
	"net/http"
)

func (h *Handler) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	downloading, err := h.store.Read().ListEpisodesByStatus(ctx, "downloading")
	if err != nil {
		h.logger.Error("queue: list downloading", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to query queue")
		return
	}
	encoding, err := h.store.Read().ListEpisodesByStatus(ctx, "encoding")
	if err != nil {
		h.logger.Error("queue: list encoding", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to query queue")
		return
	}

	// Episodes in the queue span series; cache titles to avoid a query per row.
	titles := map[int64]string{}
	seriesTitle := func(seriesID int64) string {
		if t, ok := titles[seriesID]; ok {
			return t
		}
		s, err := h.store.Read().GetSeries(ctx, seriesID)
		if err != nil {
			titles[seriesID] = ""
			return ""
		}
		titles[seriesID] = s.Title
		return s.Title
	}

	dlDetails := make([]EpisodeDetail, 0, len(downloading))
	for _, ep := range downloading {
		outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(ctx, ep.ID)
		dlDetails = append(dlDetails, episodeToDetail(ep, seriesTitle(ep.SeriesID), outputs))
	}
	encDetails := make([]EpisodeDetail, 0, len(encoding))
	for _, ep := range encoding {
		outputs, _ := h.store.Read().ListEncodedOutputsByEpisode(ctx, ep.ID)
		encDetails = append(encDetails, episodeToDetail(ep, seriesTitle(ep.SeriesID), outputs))
	}

	WriteJSON(w, http.StatusOK, QueueSnapshot{
		Downloading: dlDetails,
		Encoding:    encDetails,
	})
}
