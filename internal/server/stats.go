package server

import (
	"net/http"
)

func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := h.store.Read().ListSeriesWithProgress(ctx)
	if err != nil {
		h.logger.Error("stats: list series", "err", err)
		WriteError(w, http.StatusInternalServerError, "failed to compute stats")
		return
	}

	var seriesTotal, episodesArchived, sourceBytes, encodedBytes int64
	for _, row := range rows {
		seriesTotal++
		episodesArchived += row.EpisodeArchived
		sourceBytes += toInt64(row.SourceBytesTotal)
		encodedBytes += toInt64(row.EncodedBytesTotal)
	}

	WriteJSON(w, http.StatusOK, StatsResponse{
		SeriesTotal:       seriesTotal,
		EpisodesArchived:  episodesArchived,
		SourceBytesTotal:  sourceBytes,
		EncodedBytesTotal: encodedBytes,
		SpaceSavedBytes:   sourceBytes - encodedBytes,
	})
}
