const normalizeTierKey = (value = '') => {
  const normalized = String(value || '').trim().toLowerCase();

  if (normalized === 'primary' || normalized.includes('main') || normalized.includes('主组')) return 'primary';
  if (normalized === 'backup' || normalized.includes('secondary') || normalized.includes('备组')) return 'backup';
  if (normalized === 'cold' || normalized.includes('cold-standby') || normalized.includes('冷备')) return 'cold';
  return normalized;
};

const partitionRowsByTargetTier = (rows = [], targetTier = '') => {
  const normalizedTargetTier = normalizeTierKey(targetTier);
  const eligibleRows = [];
  const skippedRows = [];

  for (const row of Array.isArray(rows) ? rows : []) {
    const rowTier = normalizeTierKey(row?.groupKey || row?.detail?.groupKey || row?.groupLabel || '');
    if (rowTier === normalizedTargetTier) {
      skippedRows.push(row);
      continue;
    }
    eligibleRows.push(row);
  }

  return {
    eligibleRows,
    skippedRows
  };
};

export {
  normalizeTierKey,
  partitionRowsByTargetTier
};
