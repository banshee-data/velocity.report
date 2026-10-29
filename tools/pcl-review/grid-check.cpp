// Offline OQ-02 diagnostic. Fixed parameters are part of the checked-in
// protocol.
#include <algorithm>
#include <cmath>
#include <fstream>
#include <iostream>
#include <pcl/features/integral_image_normal.h>
#include <pcl/features/range_image_border_extractor.h>
#include <pcl/range_image/range_image.h>
#include <sstream>
#include <stdexcept>
#include <vector>

using Cloud = pcl::PointCloud<pcl::PointXYZ>;
// A diagnostic is unsuccessful if its evidence cannot be saved, including
// failures first reported when buffered output is closed.
std::ofstream openCSV(const std::string &path) {
  std::ofstream out;
  out.exceptions(std::ios::failbit | std::ios::badbit);
  try {
    out.open(path);
  } catch (const std::ios_base::failure &) {
    throw std::runtime_error("cannot open CSV: " + path);
  }
  return out;
}
pcl::PointCloud<pcl::Normal> normals(Cloud::Ptr cloud) {
  pcl::IntegralImageNormalEstimation<pcl::PointXYZ, pcl::Normal> e;
  e.setNormalEstimationMethod(e.COVARIANCE_MATRIX);
  e.setNormalSmoothingSize(5.0f);
  e.setDepthDependentSmoothing(false);
  e.setInputCloud(cloud);
  pcl::PointCloud<pcl::Normal> out;
  e.compute(out);
  return out;
}
bool valid(const pcl::Normal &n) {
  return std::isfinite(n.normal_x) && std::isfinite(n.normal_y) &&
         std::isfinite(n.normal_z);
}
void borders(pcl::RangeImage &range, const std::string &prefix) {
  pcl::RangeImageBorderExtractor e;
  e.setRangeImage(&range);
  pcl::PointCloud<pcl::BorderDescription> out;
  e.compute(out);
  size_t finite = 0, obstacle = 0, shadow = 0, veil = 0;
  auto csv = openCSV(prefix + "-borders.csv");
  csv << "cell,x,y,z,range,obstacle,shadow,veil\n";
  for (size_t i = 0; i < out.size(); ++i) {
    const auto &p = range[i];
    const auto &t = out[i].traits;
    bool a = t[pcl::BORDER_TRAIT__OBSTACLE_BORDER],
         b = t[pcl::BORDER_TRAIT__SHADOW_BORDER],
         c = t[pcl::BORDER_TRAIT__VEIL_POINT];
    finite += std::isfinite(p.range);
    obstacle += a;
    shadow += b;
    veil += c;
    csv << i << "," << p.x << "," << p.y << "," << p.z << "," << p.range << ","
        << a << "," << b << "," << c << "\n";
  }
  csv.close();
  std::cout << prefix << " width=" << range.width << " height=" << range.height
            << " finite=" << finite << " obstacle=" << obstacle
            << " shadow=" << shadow << " veil=" << veil << "\n";
}
int run(int argc, char **argv) {
  if (argc != 3) {
    std::cerr << "usage: pcl-grid-check frame.pcd output-prefix\n";
    return 2;
  }
  std::ifstream in(argv[1]);
  if (!in)
    return 2;
  auto cloud = Cloud::Ptr(new Cloud);
  std::vector<float> ranges;
  std::string line;
  unsigned width = 0, height = 0;
  bool ascii = false;
  while (std::getline(in, line)) {
    std::istringstream row(line);
    std::string key;
    row >> key;
    if (key == "WIDTH")
      row >> width;
    if (key == "HEIGHT")
      row >> height;
    if (key == "DATA") {
      std::string format;
      row >> format;
      ascii = format == "ascii";
      break;
    }
  }
  if (!ascii)
    throw std::runtime_error("expected DATA ascii from pcl-frame-export");
  size_t retained = 0;
  while (std::getline(in, line)) {
    std::istringstream row(line);
    std::string x, y, z, r;
    if (!(row >> x >> y >> z >> r))
      throw std::runtime_error("malformed PCD data row");
    cloud->push_back(pcl::PointXYZ(std::stof(x), std::stof(y), std::stof(z)));
    ranges.push_back(std::stof(r));
    if (std::isfinite(ranges.back())) {
      const auto &p = cloud->back();
      if (ranges.back() <= 0 || !std::isfinite(p.x) || !std::isfinite(p.y) ||
          !std::isfinite(p.z))
        throw std::runtime_error("invalid retained PCD point");
      ++retained;
    }
  }
  // push_back resets dimensions: this exporter has forty native channel rows.
  if (height != 40 || width < 5 || cloud->size() != size_t(width) * height)
    throw std::runtime_error("expected forty rows and at least five columns");
  if (retained == 0)
    throw std::runtime_error("no finite retained points");
  auto csv = openCSV(std::string(argv[2]) + "-normals.csv");
  cloud->height = height;
  cloud->width = width;
  cloud->is_dense = false;
  auto rotated = Cloud::Ptr(new Cloud(*cloud));
  for (auto &p : *rotated) {
    float y = p.y;
    p.y = -p.z;
    p.z = y;
  }
  auto a = normals(cloud), b = normals(rotated);
  size_t na = 0, nb = 0, both = 0, changed = 0;
  std::vector<double> angles;
  csv << "cell,ax,ay,az,bx,by,bz,angle_degrees\n";
  for (size_t i = 0; i < a.size(); ++i) {
    bool va = valid(a[i]), vb = valid(b[i]);
    na += va;
    nb += vb;
    changed += (va != vb);
    double angle = NAN;
    if (va && vb) {
      ++both;
      Eigen::Vector3f ar(a[i].normal_x, a[i].normal_y, a[i].normal_z),
          br(b[i].normal_x, b[i].normal_z, -b[i].normal_y);
      double dot = std::abs(ar.normalized().dot(br.normalized()));
      angle = std::acos(std::min(1.0, dot)) * 180 / M_PI;
      angles.push_back(angle);
    }
    csv << i << "," << a[i].normal_x << "," << a[i].normal_y << ","
        << a[i].normal_z << "," << b[i].normal_x << "," << b[i].normal_z << ","
        << -b[i].normal_y << "," << angle << "\n";
  }
  csv.close();
  std::sort(angles.begin(), angles.end());
  std::cout << "normals native=" << na << " rotated=" << nb
            << " common=" << both << " validity_changed=" << changed;
  if (!angles.empty())
    std::cout << " angle_p50=" << angles[angles.size() / 2]
              << " angle_p95=" << angles[size_t(.95 * (angles.size() - 1))];
  std::cout << "\n";
  pcl::RangeImage native;
  Eigen::Affine3f laser;
  pcl::RangeImage::getCoordinateFrameTransformation(
      pcl::RangeImage::LASER_FRAME, laser);
  native.setTransformationToRangeImageSystem(laser.inverse());
  native.setAngularResolution(float(.2 * M_PI / 180));
  native.width = cloud->width;
  native.height = cloud->height;
  native.resize(cloud->size());
  native.is_dense = false;
  double error = 0, dxsum = 0, dysum = 0, sqsum = 0;
  size_t count = 0;
  for (size_t i = 0; i < cloud->size(); ++i) {
    auto &p = native[i];
    p.x = (*cloud)[i].x;
    p.y = (*cloud)[i].y;
    p.z = (*cloud)[i].z;
    p.range = ranges[i];
    if (std::isfinite(p.range)) {
      float x, y;
      native.getImagePoint(p.getVector3fMap(), x, y);
      double dx = x - float(i % native.width), dy = y - float(i / native.width);
      error += std::hypot(dx, dy);
      dxsum += dx;
      dysum += dy;
      sqsum += dx * dx + dy * dy;
      ++count;
    }
  }
  std::cout << "native_projection_mean_pixel_error=" << error / count
            << " best_offset_residual_rms="
            << std::sqrt(sqsum / count - std::pow(dxsum / count, 2) -
                         std::pow(dysum / count, 2))
            << " retained=" << count << "\n";
  borders(native, std::string(argv[2]) + "-native");
  pcl::RangeImage spherical;
  spherical.createFromPointCloud(
      *cloud, float(.2 * M_PI / 180), float(2 * M_PI), float(M_PI),
      Eigen::Affine3f::Identity(), pcl::RangeImage::LASER_FRAME, 0, 0, 1);
  borders(spherical, std::string(argv[2]) + "-spherical");
  return 0;
}
int main(int argc, char **argv) {
  try {
    return run(argc, argv);
  } catch (const std::exception &e) {
    std::cerr << "pcl-grid-check: " << e.what() << "\n";
    return 1;
  }
}
