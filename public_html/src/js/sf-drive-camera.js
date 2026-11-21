// Camera poses in the game's east/north/up frame. The overhead view keeps the
// car's forward direction at the top, matching the relative emoji seeker.
export const DRIVE_VIEWS = ["zoomed", "overhead", "incar"];

export function driveCameraPose(view, car, z, ground = () => z) {
  const hx = Math.cos(car.heading),
    hy = Math.sin(car.heading);
  if (view === "overhead") {
    return {
      position: [car.x, car.y, z + 85],
      look: [car.x, car.y, z],
      up: [hx, hy, 0],
      attached: true,
      hideCar: false,
      mirror: false,
    };
  }
  if (view === "incar") {
    const slope = Math.max(
      -0.4,
      Math.min(
        0.4,
        ((ground(car.x + hx * 3, car.y + hy * 3) ?? z) -
          (ground(car.x - hx * 3, car.y - hy * 3) ?? z)) /
          6,
      ),
    );
    return {
      position: [car.x + hx * 0.35, car.y + hy * 0.35, z + 1.45 + slope * 0.35],
      look: [car.x + hx * 30, car.y + hy * 30, z + 1.45 + slope * 30],
      up: [0, 0, 1],
      attached: true,
      hideCar: true,
      mirror: true,
    };
  }
  const x = car.x - hx * 34,
    y = car.y - hy * 34;
  return {
    position: [x, y, Math.max(ground(x, y) ?? z, z) + 26],
    look: [car.x + hx * 14, car.y + hy * 14, z + 1.4],
    up: [0, 0, 1],
    attached: false,
    hideCar: false,
    mirror: false,
  };
}
