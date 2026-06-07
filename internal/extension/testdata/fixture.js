// fixture.js — deterministic test extension used by extension_test.go.
// Returns a hardcoded torrent so tests don't need network access.
export default new class FixtureProvider {
  async single({ titles, episode }) {
    return [
      {
        title: "Test Anime - " + (episode ? "E" + episode : "batch"),
        link: "magnet:?xt=urn:btih:DEADBEEFDEADBEEFDEADBEEFDEADBEEF00000000",
        hash: "DEADBEEFDEADBEEFDEADBEEFDEADBEEF00000000",
        seeders: 42,
        leechers: 7,
        downloads: 100,
        size: 1073741824,
        date: "2024-01-01T00:00:00Z",
        accuracy: "high",
        type: "main",
      }
    ];
  }

  search = this.single;
  batch  = this.single;
  movie  = this.single;

  async getSettings() {
    return {
      canSmartSearch: true,
      smartSearchFilters: ["query", "episodeNumber"],
      type: "main",
    };
  }
}
