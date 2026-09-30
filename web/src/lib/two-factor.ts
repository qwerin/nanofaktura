// Dvoufázové ověření (SPEC §3.1): pomocné funkce pro nastavení.

/** Tajný klíč pro ruční zadání po 4 znacích: „JBSW Y3DP EHPK 3PXP“. */
export function groupSecret(secret: string): string {
  return (secret.replace(/\s+/g, '').match(/.{1,4}/g) ?? []).join(' ')
}

/** Text souboru se záložními kódy (i pro schránku). */
export function recoveryCodesText(codes: string[], email?: string, now: Date = new Date()): string {
  return [
    `NanoFaktura – záložní kódy${email ? ` pro ${email}` : ''}`,
    `Vytvořeno ${now.toLocaleString('cs-CZ')}`,
    '',
    'Každý kód jde použít jen jednou, když nemáte po ruce ověřovací aplikaci ani bezpečnostní klíč.',
    '',
    ...codes,
    '',
  ].join('\n')
}
