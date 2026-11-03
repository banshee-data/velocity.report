// pcl-ground-oracle: ground0006's offline oracle. Reads one settled background exported by
// `bgasc -format pcd`, holds back every fifth point by ordinal, refits the regional ground
// surface (a port of l3grid.FitRegionalGroundSurface) on the rest, and in each 10 m region with
// its own plane fits PCL's perpendicular-plane MSAC to the region's lowest returns with
// difference-of-normals kerb candidates withheld. It writes the held-back road returns'
// residuals to both surfaces, one row per region, and the kerb lines it keeps.
//
//   pcl-ground-oracle <background.pcd> <out-dir>
//
// Outputs (the caller keeps them outside Git: the kerb lines are geometry, D-36):
//   residuals.csv  region_ix,region_iy,ordinal,x,y,z,r_regional,r_constrained
//   regions.csv    one row per region with its own regional plane
//   kerb_lines.csv one row per kept line (point on the line, direction, inliers, extent)
//   fit_check.csv  the global and per-region regional planes, for the Go cross-check
//   candidates.csv every kerb candidate in a region: ordinal, position, DoN magnitude, and
//                  whether it is a boundary cell and an inlier of its region's kept line
//   summary.txt    counts and parameters
// Every parameter is ground0006's, fixed in the spec before the run; none is a flag.

#include <pcl/features/don.h>
#include <pcl/features/normal_3d_omp.h>
#include <pcl/io/pcd_io.h>
#include <pcl/point_types.h>
#include <pcl/sample_consensus/method_types.h>
#include <pcl/sample_consensus/model_types.h>
#include <pcl/search/kdtree.h>
#include <pcl/segmentation/sac_segmentation.h>

#include <algorithm>
#include <cmath>
#include <cstdio>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <map>
#include <set>
#include <string>
#include <utility>
#include <vector>

namespace {

constexpr double kCellMetres = 10.0;
constexpr int kHoldBackEvery = 5;
constexpr double kEpsDeg = 5.0;
constexpr double kPlaneThreshold = 0.05;
constexpr int kMaxIterations = 1000;
constexpr double kSmallRadius = 0.3;
constexpr double kLargeRadius = 1.2;
constexpr double kDoNThreshold = 0.25;
constexpr int kMinFit = 12;
constexpr int kMinScored = 20;
constexpr double kBoundaryRadius = 0.3;
constexpr double kBoundaryAbove = 0.30;
constexpr double kLineThreshold = 0.10;
constexpr int kLineMinInliers = 10;
constexpr double kLineMaxTiltDeg = 20.0;
constexpr double kLineMinExtent = 2.0;

struct Plane {  // z = a x + b y + c, as l3grid.GroundSurface
  double a = 0, b = 0, c = 0;
  int support = 0;
  bool ok = false;
  double height(double x, double y) const { return a * x + b * y + c; }
};

bool finite3(const pcl::PointXYZ& p) {
  return std::isfinite(p.x) && std::isfinite(p.y) && std::isfinite(p.z);
}

// fitPlane is l3grid.fitPlane: normal equations solved by Cramer's rule.
Plane fitPlane(const std::vector<const pcl::PointXYZ*>& pts) {
  double sx = 0, sy = 0, sz = 0, sxx = 0, syy = 0, sxy = 0, sxz = 0, syz = 0;
  for (const auto* p : pts) {
    double x = p->x, y = p->y, z = p->z;
    sx += x; sy += y; sz += z; sxx += x * x; syy += y * y; sxy += x * y; sxz += x * z; syz += y * z;
  }
  double n = static_cast<double>(pts.size());
  double det = sxx * (syy * n - sy * sy) - sxy * (sxy * n - sy * sx) + sx * (sxy * sy - syy * sx);
  Plane out;
  if (std::fabs(det) < 1e-9) return out;
  double detA = sxz * (syy * n - sy * sy) - sxy * (syz * n - sy * sz) + sx * (syz * sy - syy * sz);
  double detB = sxx * (syz * n - sy * sz) - sxz * (sxy * n - sy * sx) + sx * (sxy * sz - syz * sx);
  double detC = sxx * (syy * sz - syz * sy) - sxy * (sxy * sz - syz * sx) + sxz * (sxy * sy - syy * sx);
  out.a = detA / det; out.b = detB / det; out.c = detC / det;
  out.support = static_cast<int>(pts.size());
  out.ok = true;
  return out;
}

// lowerQuartile is FitGroundSurface's cutoff: sorted heights at index (n-1)/4.
double lowerQuartile(const std::vector<const pcl::PointXYZ*>& pts) {
  std::vector<double> z;
  z.reserve(pts.size());
  for (const auto* p : pts) z.push_back(p->z);
  std::sort(z.begin(), z.end());
  return z[(z.size() - 1) / 4];
}

// fitGround is l3grid.FitGroundSurface.
Plane fitGround(const std::vector<const pcl::PointXYZ*>& in) {
  std::vector<const pcl::PointXYZ*> usable;
  for (const auto* p : in)
    if (finite3(*p)) usable.push_back(p);
  if (usable.size() < 12) return {};
  double cutoff = lowerQuartile(usable);
  std::vector<const pcl::PointXYZ*> cand;
  for (const auto* p : usable)
    if (p->z <= cutoff) cand.push_back(p);
  if (cand.size() < 12) return {};
  Plane first = fitPlane(cand);
  if (!first.ok) return {};
  std::vector<double> res;
  for (const auto* p : cand) res.push_back(std::fabs(p->z - first.height(p->x, p->y)));
  std::sort(res.begin(), res.end());
  double threshold = std::max(0.05, 3 * res[res.size() / 2]);
  std::vector<const pcl::PointXYZ*> inl;
  for (const auto* p : cand)
    if (std::fabs(p->z - first.height(p->x, p->y)) <= threshold) inl.push_back(p);
  if (inl.size() < 12) return {};
  return fitPlane(inl);
}

using Key = std::pair<int, int>;
Key keyFor(double x, double y) {
  return {static_cast<int>(std::floor(x / kCellMetres)), static_cast<int>(std::floor(y / kCellMetres))};
}

}  // namespace

int main(int argc, char** argv) {
  if (argc != 3) {
    std::cerr << "usage: pcl-ground-oracle <background.pcd> <out-dir>\n";
    return 2;
  }
  const std::string out = argv[2];
  pcl::PointCloud<pcl::PointXYZ>::Ptr cloud(new pcl::PointCloud<pcl::PointXYZ>);
  if (pcl::io::loadPCDFile<pcl::PointXYZ>(argv[1], *cloud) != 0 || cloud->empty()) {
    std::cerr << "cannot read " << argv[1] << "\n";
    return 1;
  }
  const size_t n = cloud->size();
  std::vector<bool> held(n);
  pcl::PointCloud<pcl::PointXYZ>::Ptr fitCloud(new pcl::PointCloud<pcl::PointXYZ>);
  std::vector<const pcl::PointXYZ*> fitPts;
  for (size_t i = 0; i < n; ++i) {
    held[i] = (i % kHoldBackEvery) == 0;
    if (!held[i] && finite3((*cloud)[i])) fitCloud->push_back((*cloud)[i]);
  }
  for (const auto& p : *fitCloud) fitPts.push_back(&p);

  // The regional surface on the fit points, as l3grid.FitRegionalGroundSurface.
  Plane global = fitGround(fitPts);
  if (!global.ok) {
    std::cerr << "the global regional fit failed\n";
    return 1;
  }
  std::map<Key, std::vector<const pcl::PointXYZ*>> fitByCell;
  for (const auto* p : fitPts) fitByCell[keyFor(p->x, p->y)].push_back(p);
  std::map<Key, Plane> regional;
  for (const auto& [k, pts] : fitByCell) {
    Plane pl = fitGround(pts);
    if (pl.ok) regional[k] = pl;
  }

  // Kerb candidates: DoN on every point, normals from the fit points, flipped towards the
  // sensor origin (NormalEstimation's default viewpoint).
  pcl::search::KdTree<pcl::PointXYZ>::Ptr tree(new pcl::search::KdTree<pcl::PointXYZ>);
  pcl::PointCloud<pcl::PointNormal>::Ptr nSmall(new pcl::PointCloud<pcl::PointNormal>);
  pcl::PointCloud<pcl::PointNormal>::Ptr nLarge(new pcl::PointCloud<pcl::PointNormal>);
  {
    pcl::NormalEstimationOMP<pcl::PointXYZ, pcl::PointNormal> ne(2);
    ne.setInputCloud(cloud);
    ne.setSearchSurface(fitCloud);
    ne.setSearchMethod(tree);
    ne.setViewPoint(0.0f, 0.0f, 0.0f);
    ne.setRadiusSearch(kSmallRadius);
    ne.compute(*nSmall);
    ne.setRadiusSearch(kLargeRadius);
    ne.compute(*nLarge);
  }
  pcl::PointCloud<pcl::PointNormal>::Ptr don(new pcl::PointCloud<pcl::PointNormal>);
  pcl::copyPointCloud(*cloud, *don);
  {
    pcl::DifferenceOfNormalsEstimation<pcl::PointXYZ, pcl::PointNormal, pcl::PointNormal> est;
    est.setInputCloud(cloud);
    est.setNormalScaleSmall(nSmall);
    est.setNormalScaleLarge(nLarge);
    if (!est.initCompute()) {
      std::cerr << "DoN initCompute failed\n";
      return 1;
    }
    est.computeFeature(*don);
  }
  std::vector<bool> kerb(n, false);
  size_t donFinite = 0, kerbCount = 0;
  for (size_t i = 0; i < n; ++i) {
    float m = (*don)[i].curvature;  // DoN writes |(n_small - n_large) / 2| here
    if (std::isfinite(m)) {
      ++donFinite;
      if (m > kDoNThreshold) { kerb[i] = true; ++kerbCount; }
    }
  }

  // Points of every region, by ordinal.
  std::map<Key, std::vector<int>> byCell;
  for (size_t i = 0; i < n; ++i)
    if (finite3((*cloud)[i])) byCell[keyFor((*cloud)[i].x, (*cloud)[i].y)].push_back(static_cast<int>(i));

  pcl::search::KdTree<pcl::PointXYZ> all;
  all.setInputCloud(cloud);

  std::ofstream res(out + "/residuals.csv"), reg(out + "/regions.csv"), lines(out + "/kerb_lines.csv"),
      chk(out + "/fit_check.csv"), cand(out + "/candidates.csv");
  for (const auto* f : {&res, &reg, &lines, &chk, &cand}) {
    if (!*f) {
      std::cerr << "cannot write to " << out << ": the output directory must exist\n";
      return 1;
    }
  }
  cand << std::setprecision(9) << "region_ix,region_iy,ordinal,x,y,z,don,boundary,line_inlier\n";
  res << std::setprecision(9) << "region_ix,region_iy,ordinal,x,y,z,r_regional,r_constrained\n";
  reg << std::setprecision(9)
      << "region_ix,region_iy,regional_a,regional_b,regional_c,regional_support,cutoff_z,fit_points,"
         "kerb_candidates,supported,reason,nx,ny,nz,d,tilt_deg,inliers,mask_mismatch,held_road,"
         "holds_kerb\n";
  lines << std::setprecision(9) << "region_ix,region_iy,px,py,pz,dx,dy,dz,inliers,extent_m,tilt_deg\n";
  chk << std::setprecision(12) << "region_ix,region_iy,a,b,c,support\n";
  chk << "global,global," << global.a << "," << global.b << "," << global.c << "," << global.support << "\n";

  int supported = 0, unsupported = 0, mismatchRegions = 0, kerbRegions = 0;
  for (const auto& [k, pl] : regional) {
    chk << k.first << "," << k.second << "," << pl.a << "," << pl.b << "," << pl.c << "," << pl.support << "\n";
    const auto& idx = byCell[k];
    std::vector<const pcl::PointXYZ*> cellFit;
    for (int i : idx)
      if (!held[i]) cellFit.push_back(&(*cloud)[i]);
    double cutoff = lowerQuartile(cellFit);
    pcl::Indices fitIdx;
    int candidates = 0;
    for (int i : idx) {
      if (kerb[i]) ++candidates;
      if (!held[i] && !kerb[i] && (*cloud)[i].z <= cutoff) fitIdx.push_back(i);
    }
    bool ok = false;
    std::string reason;
    double nx = NAN, ny = NAN, nz = NAN, d = NAN, tilt = NAN;
    int inlierCount = 0, mismatch = 0;
    if (static_cast<int>(fitIdx.size()) < kMinFit) {
      reason = "few_fit_points";
    } else {
      pcl::SACSegmentation<pcl::PointXYZ> seg;
      seg.setOptimizeCoefficients(true);
      seg.setModelType(pcl::SACMODEL_PERPENDICULAR_PLANE);
      seg.setMethodType(pcl::SAC_MSAC);
      seg.setAxis(Eigen::Vector3f(0, 0, 1));
      seg.setEpsAngle(kEpsDeg * M_PI / 180.0);
      seg.setDistanceThreshold(kPlaneThreshold);
      seg.setMaxIterations(kMaxIterations);
      seg.setInputCloud(cloud);
      pcl::IndicesPtr ip(new pcl::Indices(fitIdx));
      seg.setIndices(ip);
      pcl::PointIndices inl;
      pcl::ModelCoefficients co;
      seg.segment(inl, co);
      if (inl.indices.empty() || co.values.size() != 4) {
        reason = "empty_fit";
      } else {
        double norm = std::sqrt(co.values[0] * co.values[0] + co.values[1] * co.values[1] +
                                co.values[2] * co.values[2]);
        double s = (co.values[2] < 0 ? -1.0 : 1.0) / norm;
        nx = co.values[0] * s; ny = co.values[1] * s; nz = co.values[2] * s; d = co.values[3] * s;
        tilt = std::acos(std::min(1.0, std::fabs(nz))) * 180.0 / M_PI;
        if (std::fabs(nz) < 1e-6) {
          reason = "vertical_plane";
        } else {
          ok = true;
          inlierCount = static_cast<int>(inl.indices.size());
          std::set<int> returned(inl.indices.begin(), inl.indices.end());
          for (int i : fitIdx) {
            const auto& p = (*cloud)[i];
            bool in = std::fabs(nx * p.x + ny * p.y + nz * p.z + d) < kPlaneThreshold;
            if (in != (returned.count(i) > 0)) ++mismatch;
          }
          if (mismatch) ++mismatchRegions;
        }
      }
    }
    ok ? ++supported : ++unsupported;

    int heldRoad = 0;
    for (int i : idx) {
      const auto& p = (*cloud)[i];
      if (!held[i] || kerb[i] || p.z > cutoff) continue;
      ++heldRoad;
      double rReg = p.z - pl.height(p.x, p.y);
      double rCon = ok ? p.z - (-(nx * p.x + ny * p.y + d) / nz) : rReg;
      res << k.first << "," << k.second << "," << i << "," << p.x << "," << p.y << "," << p.z << ","
          << rReg << "," << rCon << "\n";
    }

    // Kerb lines on the region's boundary candidates.
    bool holds = false;
    pcl::Indices boundary;
    for (int i : idx) {
      const auto& p = (*cloud)[i];
      if (!kerb[i] || p.z > cutoff + kBoundaryAbove) continue;
      pcl::Indices nb;
      std::vector<float> dist;
      all.radiusSearch(p, kBoundaryRadius, nb, dist);
      for (int j : nb)
        if (!kerb[j]) { boundary.push_back(i); break; }
    }
    std::set<int> lineInliers;
    if (static_cast<int>(boundary.size()) >= kLineMinInliers) {
      pcl::SACSegmentation<pcl::PointXYZ> ls;
      ls.setOptimizeCoefficients(true);
      ls.setModelType(pcl::SACMODEL_LINE);
      ls.setMethodType(pcl::SAC_RANSAC);
      ls.setDistanceThreshold(kLineThreshold);
      ls.setMaxIterations(kMaxIterations);
      ls.setInputCloud(cloud);
      pcl::IndicesPtr bp(new pcl::Indices(boundary));
      ls.setIndices(bp);
      pcl::PointIndices li;
      pcl::ModelCoefficients lc;
      ls.segment(li, lc);
      if (static_cast<int>(li.indices.size()) >= kLineMinInliers && lc.values.size() == 6) {
        Eigen::Vector3d p0(lc.values[0], lc.values[1], lc.values[2]);
        Eigen::Vector3d dir(lc.values[3], lc.values[4], lc.values[5]);
        dir.normalize();
        double lineTilt = std::asin(std::min(1.0, std::fabs(dir.z()))) * 180.0 / M_PI;
        double lo = 1e18, hi = -1e18;
        for (int i : li.indices) {
          const auto& p = (*cloud)[i];
          double t = (Eigen::Vector3d(p.x, p.y, p.z) - p0).dot(dir);
          lo = std::min(lo, t); hi = std::max(hi, t);
        }
        double extent = hi - lo;
        if (lineTilt <= kLineMaxTiltDeg && extent >= kLineMinExtent) {
          holds = true;
          ++kerbRegions;
          lineInliers.insert(li.indices.begin(), li.indices.end());
          lines << k.first << "," << k.second << "," << p0.x() << "," << p0.y() << "," << p0.z() << ","
                << dir.x() << "," << dir.y() << "," << dir.z() << "," << li.indices.size() << "," << extent
                << "," << lineTilt << "\n";
        }
      }
    }
    std::set<int> boundarySet(boundary.begin(), boundary.end());
    for (int i : idx) {
      if (!kerb[i]) continue;
      const auto& p = (*cloud)[i];
      cand << k.first << "," << k.second << "," << i << "," << p.x << "," << p.y << "," << p.z << ","
           << (*don)[i].curvature << "," << boundarySet.count(i) << "," << lineInliers.count(i) << "\n";
    }
    reg << k.first << "," << k.second << "," << pl.a << "," << pl.b << "," << pl.c << "," << pl.support << ","
        << cutoff << "," << fitIdx.size() << "," << candidates << "," << (ok ? 1 : 0) << "," << reason << "," << nx
        << "," << ny << "," << nz << "," << d << "," << tilt << "," << inlierCount << "," << mismatch << ","
        << heldRoad << "," << (holds ? 1 : 0) << "\n";
  }

  std::ofstream sum(out + "/summary.txt");
  if (!sum) {
    std::cerr << "cannot write " << out << "/summary.txt\n";
    return 1;
  }
  sum << "points " << n << "\nheld_back " << (n + kHoldBackEvery - 1) / kHoldBackEvery << "\nfit_points "
      << fitCloud->size() << "\nglobal_a " << global.a << "\nglobal_b " << global.b << "\nglobal_c " << global.c
      << "\nregions " << regional.size() << "\nsupported " << supported << "\nunsupported " << unsupported
      << "\nmask_mismatch_regions " << mismatchRegions << "\nkerb_regions " << kerbRegions << "\ndon_finite "
      << donFinite << "\nkerb_candidates " << kerbCount << "\noptimise_coefficients 1\nmethod MSAC\neps_deg "
      << kEpsDeg << "\nthreshold_m " << kPlaneThreshold << "\nmax_iterations " << kMaxIterations
      << "\npcl_version " << PCL_VERSION_PRETTY << "\n";
  for (auto* f : {&res, &reg, &lines, &chk, &cand, &sum}) {
    f->flush();
    if (!*f) {
      std::cerr << "a write to " << out << " failed\n";
      return 1;
    }
  }
  std::cout << argv[1] << ": " << n << " points, " << regional.size() << " regions, " << supported
            << " supported, " << kerbRegions << " with a kerb line, " << kerbCount << " kerb candidates\n";
  return 0;
}
