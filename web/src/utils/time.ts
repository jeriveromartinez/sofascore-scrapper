export function formatUnixTimestamp(value: number): string {
  if (!value) return "";
  const millis = value < 1_000_000_000_000 ? value * 1000 : value;
  return new Date(millis).toLocaleString("en-GB");
}
