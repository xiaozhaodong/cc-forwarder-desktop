export const calculateEndpointStats = (endpoints) => {
  if (!endpoints || endpoints.length === 0) {
    return { total: 0, healthy: 0, unhealthy: 0, unchecked: 0, healthPercentage: 0 };
  }

  const healthy = endpoints.filter(e => e.healthy && !e.never_checked).length;
  const unhealthy = endpoints.filter(e => !e.healthy && !e.never_checked).length;
  const unchecked = endpoints.filter(e => e.never_checked).length;
  const total = endpoints.length;

  return {
    total,
    healthy,
    unhealthy,
    unchecked,
    healthPercentage: healthy + unhealthy > 0 ? ((healthy / (healthy + unhealthy)) * 100).toFixed(1) : 0
  };
};

export const applyEndpointHealthCheckResult = (endpoints = [], endpointName, result = {}, checkedAt = new Date().toISOString()) => {
  return endpoints.map(ep =>
    ep.name === endpointName
      ? {
          ...ep,
          healthy: result.healthy !== false,
          never_checked: false,
          last_check: checkedAt,
          response_time: result.response_time || ep.response_time
        }
      : ep
  );
};
