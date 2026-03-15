export const getEndpointLastCheckDisplayValue = (endpoint) => {
  if (!endpoint) return '-';

  if (endpoint.never_checked === true || endpoint.neverChecked === true) {
    return '-';
  }

  return endpoint.lastCheck || endpoint.last_check || '-';
};
