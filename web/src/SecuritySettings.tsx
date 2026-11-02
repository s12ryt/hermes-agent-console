import { useEffect, useState } from "react";
type Props = {
  request: (path: string, method?: string, body?: unknown) => Promise<any>;
};
export default function SecuritySettings({ request }: Props) {
  const [keys, setKeys] = useState<{ id: number; name: string }[]>([]);
  const [rules, setRules] = useState<{ id: number; pattern: string }[]>([]);
  const [name, setName] = useState("openrouter"),
    [value, setValue] = useState(""),
    [pattern, setPattern] = useState("");
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const refresh = async () => {
    const [k, r] = await Promise.all([request("/keys"), request("/rules")]);
    setKeys(Array.isArray(k) ? k : []);
    setRules(Array.isArray(r) ? r : []);
  };
  useEffect(() => {
    let alive = true;
    Promise.all([request("/keys"), request("/rules")])
      .then(([k, r]) => {
        if (alive) {
          setKeys(Array.isArray(k) ? k : []);
          setRules(Array.isArray(r) ? r : []);
        }
      })
      .catch((e) => {
        if (alive) setError(String(e.message ?? e));
      });
    return () => {
      alive = false;
    };
  }, [request]);
  const change = async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await action();
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel">
      <h2>金鑰與自動核准</h2>
      {error && <p role="alert">{error}</p>}
      <p>金鑰加密儲存，重新連線後生效。管理密碼變更後需重新設定金鑰。</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void change(async () => {
            await request("/keys", "POST", { name, value });
            setValue("");
          });
        }}
      >
        <label>
          供應商
          <select value={name} onChange={(e) => setName(e.target.value)}>
            {["openrouter", "openai", "anthropic", "gemini"].map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </label>
        <label>
          供應商 API Key
          <input
            type="password"
            autoComplete="new-password"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            required
            maxLength={8192}
          />
        </label>
        <button disabled={busy || !value}>儲存金鑰</button>
      </form>
      <ul>
        {keys.map((k) => (
          <li key={k.id}>
            {k.name} · 已設定{" "}
            <button
              disabled={busy}
              aria-label={`刪除 ${k.name} 金鑰`}
              onClick={() =>
                void change(async () => {
                  await request("/keys/delete", "POST", { id: k.id });
                })
              }
            >
              刪除
            </button>
          </li>
        ))}
      </ul>
      <p>
        只會自動允許完全相符的命令；不支援前綴或含 shell
        運算子的規則。加入前請確認該命令在工作目錄中的效果。
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void change(async () => {
            await request("/rules", "POST", { pattern });
            setPattern("");
          });
        }}
      >
        <label>
          完整核准命令
          <input
            value={pattern}
            onChange={(e) => setPattern(e.target.value)}
            required
            maxLength={256}
          />
        </label>
        <button disabled={busy || !pattern.trim()}>新增核准規則</button>
      </form>
      <ul>
        {rules.map((r) => (
          <li key={r.id}>
            <code>{r.pattern}</code>{" "}
            <button
              disabled={busy}
              aria-label={`刪除規則 ${r.pattern}`}
              onClick={() =>
                void change(async () => {
                  await request("/rules/delete", "POST", { id: r.id });
                })
              }
            >
              刪除
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
