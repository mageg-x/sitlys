export function defaultFunnelSteps() {
  return [
    { label: "Landing", type: "page", value: "/" },
    { label: "Signup", type: "event", value: "signup" },
  ];
}
