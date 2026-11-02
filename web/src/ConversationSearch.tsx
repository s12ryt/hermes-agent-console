import { useState, type FormEvent } from "react";
type Hit = { session_id: string; text: string };
export default function ConversationSearch({
  request,
  open,
}: {
  request: (path: string) => Promise<Hit[]>;
  open: (id: string) => Promise<void>;
}) {
  const [query, setQuery] = useState(""),
    [hits, setHits] = useState<Hit[]>([]),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [searched, setSearched] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      setHits(await request("/search?q=" + encodeURIComponent(query)));
      setSearched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "搜尋失敗");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-label="對話全文搜尋">
      <form onSubmit={submit}>
        <input
          aria-label="搜尋對話內容"
          value={query}
          maxLength={256}
          onChange={(e) => setQuery(e.target.value)}
        />
        <button disabled={busy || !query.trim()}>搜尋內容</button>
      </form>
      <small>搜尋已載入歷史的本機索引。</small>
      {error && <p role="alert">{error}</p>}
      {searched && !hits.length && <p role="status">沒有符合的對話</p>}
      {hits.map((hit) => (
        <button
          key={hit.session_id}
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            try {
              await open(hit.session_id);
            } catch (e) {
              setError(e instanceof Error ? e.message : "無法開啟對話");
            } finally {
              setBusy(false);
            }
          }}
        >
          {hit.session_id}
          <span>{hit.text}</span>
        </button>
      ))}
    </section>
  );
}
