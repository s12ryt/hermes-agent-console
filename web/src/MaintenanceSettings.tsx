import { useState } from "react";
function UpdateCheck({ request }: { request: (path: string) => Promise<any> }) {
  const [result, setResult] = useState<{
      current: string;
      latest: string;
      url: string;
    }>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const check = async () => {
    setBusy(true);
    setError("");
    try {
      setResult(await request("/updates"));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <h2>面板版本</h2>
      <button disabled={busy} onClick={() => void check()}>
        檢查面板更新
      </button>
      {result && (
        <p>
          目前 {result.current} ·{" "}
          <a href={result.url} target="_blank" rel="noreferrer">
            查看 {result.latest}
          </a>
        </p>
      )}
      {error && <p role="alert">{error}</p>}
    </>
  );
}
export default function MaintenanceSettings({
  request,
}: {
  request: (path: string, method?: string, body?: unknown) => Promise<any>;
}) {
  const [backup, setBackup] = useState<unknown>(),
    [confirmed, setConfirmed] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");
  const read = async (file?: File) => {
    setBackup(undefined);
    setConfirmed(false);
    setNotice("");
    setError("");
    if (!file) return;
    if (file.size > 1024 * 1024) {
      setError("備份需小於 1 MiB");
      return;
    }
    try {
      const text = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.onerror = () => reject(new Error("無法讀取備份"));
        reader.readAsText(file);
      });
      const data = JSON.parse(text);
      if (
        data?.version !== 1 ||
        !data.settings ||
        !Array.isArray(data.approval_rules) ||
        !Array.isArray(data.encrypted_keys)
      )
        throw new Error();
      setBackup(data);
    } catch {
      setError("請選擇有效的 JSON 設定備份");
    }
  };
  const restore = async () => {
    setBusy(true);
    setError("");
    try {
      await request("/restore", "POST", backup);
      setBackup(undefined);
      setConfirmed(false);
      setNotice("還原完成，重新開啟設定頁以載入還原的設定");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel">
      <UpdateCheck request={request} />
      <h2>備份與還原</h2>
      <p>
        包含面板設定、核准規則與加密金鑰；不包含 Hermes
        對話、工作目錄、操作紀錄與用量。還原金鑰需使用相同管理密碼，請先停止
        Hermes。
      </p>
      <a href="/api/backup" download="hermes-console-backup.json">
        下載設定備份
      </a>
      <label>
        選擇設定備份
        <input
          type="file"
          accept=".json,application/json"
          disabled={busy}
          onChange={(e) => void read(e.target.files?.[0])}
        />
      </label>
      <label>
        <input
          type="checkbox"
          checked={confirmed}
          disabled={busy}
          onChange={(e) => setConfirmed(e.target.checked)}
        />
        我了解還原會覆寫目前設定、規則與金鑰
      </label>
      <button
        disabled={!backup || !confirmed || busy}
        onClick={() => void restore()}
      >
        還原設定備份
      </button>
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
    </section>
  );
}
