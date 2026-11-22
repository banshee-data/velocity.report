export function sampleFlightProfile(car, road, heightAt) {
  const heading = car.heading + (car.v < 0 ? Math.PI : 0);
  const direction = road && Math.cos(heading - road.heading) < 0 ? -1 : 1;
  const hx = Math.cos(heading),
    hy = Math.sin(heading);
  const profile = [];
  const ahead = Math.max(80, Math.abs(car.v) * 7);
  let stations, entry;
  if (road) {
    stations = [];
    for (let i = 0, distance = 0; i < road.centreline.length; i += 2) {
      const x = road.origin[0] + road.centreline[i] / 10;
      const y = road.origin[1] + road.centreline[i + 1] / 10;
      if (stations.length) {
        const last = stations.at(-1);
        distance += Math.hypot(x - last.x, y - last.y);
      }
      stations.push({ x, y, distance });
    }
    const a = stations[road.segment / 2];
    const b = stations[road.segment / 2 + 1];
    entry = a.distance + road.fraction * (b.distance - a.distance);
  }
  for (let s = -24; s <= ahead; s += 2) {
    let x = car.x + hx * s,
      y = car.y + hy * s;
    if (stations) {
      const distance = entry + s * direction;
      const i = stations.findIndex((point) => point.distance > distance);
      if (i < 1) {
        profile.push({ s, z: null });
        continue;
      }
      const a = stations[i - 1],
        b = stations[i];
      const t = (distance - a.distance) / (b.distance - a.distance);
      x = a.x + (b.x - a.x) * t;
      y = a.y + (b.y - a.y) * t;
    }
    profile.push({ s, z: heightAt(x, y) });
  }
  function stationAt(x, y) {
    if (!stations) return (x - car.x) * hx + (y - car.y) * hy;
    let best = Infinity,
      station = entry;
    for (let i = 1; i < stations.length; i++) {
      const a = stations[i - 1],
        b = stations[i];
      const dx = b.x - a.x,
        dy = b.y - a.y,
        length = b.distance - a.distance;
      if (length === 0) continue;
      const projection = ((x - a.x) * dx + (y - a.y) * dy) / (length * length);
      const t = Math.max(
        i === 1 ? -Infinity : 0,
        Math.min(i === stations.length - 1 ? Infinity : 1, projection),
      );
      const d = Math.hypot(x - a.x - t * dx, y - a.y - t * dy);
      if (d < best) {
        best = d;
        station = a.distance + t * length;
      }
    }
    return (station - entry) * direction;
  }
  return {
    profile,
    stationAt,
    basis: road ? "Road centreline" : "Takeoff heading",
  };
}

export function createAirtimeHud(panel, readout, canvas) {
  const ctx = canvas.getContext("2d");
  let flight;
  let shownUntil = 0;
  function reset() {
    flight = undefined;
    shownUntil = 0;
    panel.hidden = true;
  }
  function step(before, car, dt, road, heightAt) {
    if (car.grounded !== false && !flight) return;
    if (!flight) {
      flight = {
        ...sampleFlightProfile(before, road, heightAt),
        trace: [{ s: 0, z: before.z }],
        seconds: 0,
        peak: 0,
        landed: false,
      };
    }
    if (flight.landed && car.grounded !== false) return;
    if (flight.landed) {
      reset();
      step(before, car, dt, road, heightAt);
      return;
    }
    const z = heightAt(car.x, car.y);
    if (z !== null) flight.peak = Math.max(flight.peak, car.z - z);
    const s = flight.stationAt(car.x, car.y);
    flight.trace.push({ s, z: car.z });
    flight.seconds += dt;
    flight.landed = car.grounded !== false;
    if (flight.landed) {
      if (flight.peak < 0.18) reset();
      else shownUntil = performance.now() + 1700;
    }
  }
  function draw(now) {
    if (!flight || flight.peak < 0.18 || (flight.landed && now > shownUntil)) {
      panel.hidden = true;
      return;
    }
    panel.hidden = false;
    const last = flight.trace.at(-1);
    readout.textContent = `${flight.landed ? "Touchdown" : "Airborne"} · ${flight.seconds.toFixed(1)} s · ${last.z.toFixed(1)} m elevation`;
    const left = Math.min(-16, last.s - 56),
      right = Math.max(55, last.s + 45);
    const profile = flight.profile.filter(({ s }) => s >= left && s <= right);
    const visible = [...profile, ...flight.trace].filter(
      ({ s, z }) => z !== null && s >= left && s <= right,
    );
    const lo = Math.min(...visible.map(({ z }) => z)) - 1;
    const hi = Math.max(...visible.map(({ z }) => z)) + 1;
    const w = canvas.width,
      h = canvas.height;
    const xAt = (s) => 14 + ((w - 28) * (s - left)) / (right - left);
    const yAt = (z) => 16 + ((h - 54) * (hi - z)) / (hi - lo);
    ctx.clearRect(0, 0, w, h);
    const plot = (points, colour) => {
      ctx.beginPath();
      let pen = false;
      for (const { s, z } of points) {
        if (z === null || s < left || s > right) {
          pen = false;
          continue;
        }
        if (pen) ctx.lineTo(xAt(s), yAt(z));
        else ctx.moveTo(xAt(s), yAt(z));
        pen = true;
      }
      ctx.strokeStyle = colour;
      ctx.lineWidth = 4;
      ctx.stroke();
    };
    plot(profile, "#38bdf8");
    plot(flight.trace, "#f59e0b");
    ctx.beginPath();
    ctx.arc(xAt(last.s), yAt(last.z), 6, 0, Math.PI * 2);
    ctx.fillStyle = "#f59e0b";
    ctx.fill();
    ctx.fillStyle = getComputedStyle(panel).color;
    ctx.font = "20px system-ui";
    ctx.fillText(`${flight.basis} · metres from takeoff`, 14, h - 10);
    ctx.fillText(`${Math.round(left)}`, 14, h - 32);
    ctx.textAlign = "right";
    ctx.fillText(`${Math.round(right)}`, w - 14, h - 32);
    ctx.textAlign = "left";
  }
  return { step, reset, draw };
}
