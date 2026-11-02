import { useRef, useState } from "react";
type Listing = {
  path: string;
  entries?: { name: string; dir: boolean; symlink?: boolean }[];
  content?: string;
  truncated?: boolean;
};
export default function WorkspaceFiles({
  request,
}: {
  request: (path: string) => Promise<Listing>;
}) {
  const [data, setData] = useState<Listing | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const sequence = useRef(0);
  const open = async (path: string) => {
    const id = ++sequence.current;
    setBusy(true);
    setError("");
    try {
      const result = await request("/files?path=" + encodeURIComponent(path));
      if (id === sequence.current) setData(result);
    } catch (e) {
      if (id === sequence.current)
        setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (id === sequence.current) setBusy(false);
    }
  };
  const parent = data?.path.split("/").slice(0, -1).join("/") || ".";
  return (
    <section className="panel">
      <h2>工作目錄檔案</h2>
      <p>可預覽 256 KiB 內的文字檔。</p>
      <button disabled={busy} onClick={() => void open(".")}>
        瀏覽工作目錄
      </button>
      {data && (
        <>
          <code>{data.path}</code>
          <button
            disabled={busy || data.path === "."}
            onClick={() => void open(parent)}
          >
            上一層
          </button>
        </>
      )}
      {error && <p role="alert">{error}</p>}
      {data?.truncated && <p>只顯示前 500 筆項目</p>}
      {data?.entries && (
        <ul>
          {[...data.entries]
            .sort(
              (a, b) =>
                Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name),
            )
            .map((entry) => (
              <li key={entry.name}>
                <button
                  disabled={busy}
                  aria-label={`開啟 ${entry.name}`}
                  onClick={() =>
                    void open(
                      data.path === "."
                        ? entry.name
                        : `${data.path}/${entry.name}`,
                    )
                  }
                >
                  {entry.dir ? "📁" : "📄"} {entry.name}
                  {entry.symlink ? "（連結）" : ""}
                </button>
              </li>
            ))}
        </ul>
      )}
      {data?.content !== undefined && (
        <pre className="file-preview">{data.content}</pre>
      )}
    </section>
  );
}
