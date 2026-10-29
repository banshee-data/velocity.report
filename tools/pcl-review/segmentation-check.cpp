// Small, hand-authored contract fixtures for reviewed PCL; no capture input.
#include <algorithm>
#include <cmath>
#include <iostream>
#include <pcl/filters/morphological_filter.h>
#include <pcl/segmentation/impl/progressive_morphological_filter.hpp>
#include <pcl/segmentation/impl/region_growing.hpp>
#include <pcl/segmentation/impl/sac_segmentation.hpp>
#include <stdexcept>

using Point = pcl::PointXYZ;
using Cloud = pcl::PointCloud<Point>;

void require(bool ok, const char *message) {
  if (!ok)
    throw std::runtime_error(message);
}

std::size_t regions(unsigned k, float height) {
  auto cloud = pcl::make_shared<Cloud>();
  for (auto p : {Point(0, 0, 0), Point(.1f, 0, 0), Point(100, 0, height),
                 Point(100.1f, 0, height)})
    cloud->push_back(p);
  auto normals = pcl::make_shared<pcl::PointCloud<pcl::Normal>>();
  for (int i = 0; i < 4; ++i) {
    pcl::Normal n;
    n.normal_x = n.normal_y = n.curvature = 0;
    n.normal_z = 1;
    normals->push_back(n);
  }
  pcl::RegionGrowing<Point, pcl::Normal> grow;
  grow.setInputCloud(cloud);
  grow.setInputNormals(normals);
  grow.setMinClusterSize(1);
  grow.setMaxClusterSize(10);
  grow.setNumberOfNeighbours(k);
  grow.setSmoothnessThreshold(.01f);
  grow.setCurvatureThreshold(.01f);
  grow.setResidualTestFlag(true);
  grow.setResidualThreshold(.001f);
  std::vector<pcl::PointIndices> clusters;
  grow.extract(clusters);
  std::cout << "region k=" << k << " height=" << height
            << " clusters=" << clusters.size() << '\n';
  return clusters.size();
}

void morphology() {
  for (auto heights : {std::vector<float>{0, 1, 1}, {0, 1, 0}, {2, 2, 2}}) {
    auto cloud = pcl::make_shared<Cloud>();
    for (std::size_t i = 0; i < heights.size(); ++i)
      cloud->push_back(Point(static_cast<float>(i), 0, heights[i]));
    Cloud eroded, opened;
    pcl::applyMorphologicalOperator<Point>(cloud, 3, pcl::MORPH_ERODE, eroded);
    pcl::applyMorphologicalOperator<Point>(cloud, 3, pcl::MORPH_OPEN, opened);
    const std::vector<float> expected = heights == std::vector<float>{0, 1, 0}
                                            ? std::vector<float>{0, 0, 0}
                                            : heights;
    for (std::size_t i = 0; i < heights.size(); ++i)
      require(opened[i].z == expected[i], "opening fixture failed");
    if (heights == std::vector<float>{0, 1, 1})
      require(eroded[1].z == 0 && opened[1].z == 1,
              "opening must differ from erosion");
    pcl::ProgressiveMorphologicalFilter<Point> pmf;
    pmf.setInputCloud(cloud);
    pmf.setCellSize(1);
    pmf.setMaxWindowSize(3);
    pmf.setInitialDistance(1);
    pmf.setMaxDistance(1);
    pcl::Indices ground;
    pmf.extract(ground);
    const pcl::Indices expected_indices = heights == std::vector<float>{0, 1, 0}
                                              ? pcl::Indices{0, 2}
                                              : pcl::Indices{0, 1, 2};
    require(ground == expected_indices, "PMF membership fixture failed");
    std::cout << "morph heights=";
    for (float z : heights)
      std::cout << z << ',';
    std::cout << " eroded=";
    for (auto p : eroded)
      std::cout << p.z << ',';
    std::cout << " opened=";
    for (auto p : opened)
      std::cout << p.z << ',';
    std::cout << " ground=";
    for (auto i : ground)
      std::cout << i << ',';
    std::cout << '\n';
  }
}

void membership() {
  constexpr double threshold = .027;
  auto cloud = pcl::make_shared<Cloud>();
  for (int i = 0; i < 25; ++i)
    cloud->push_back(
        Point(i % 5, i / 5, ((i * i * 7 + i * 3) % 17 - 8) * .004f));
  pcl::PointIndices initial, refined;
  pcl::ModelCoefficients initial_coeff, refined_coeff;
  for (bool optimise : {false, true}) {
    pcl::SACSegmentation<Point> seg;
    seg.setInputCloud(cloud);
    seg.setModelType(pcl::SACMODEL_PERPENDICULAR_PLANE);
    seg.setAxis(Eigen::Vector3f::UnitZ());
    seg.setEpsAngle(5 * std::acos(-1.0) / 180);
    seg.setMethodType(pcl::SAC_MSAC);
    seg.setDistanceThreshold(threshold);
    seg.setMaxIterations(1000);
    seg.setOptimizeCoefficients(optimise);
    seg.segment(optimise ? refined : initial,
                optimise ? refined_coeff : initial_coeff);
  }
  require(refined_coeff.values.size() == 4, "no refined plane");
  pcl::Indices expected;
  for (std::size_t i = 0; i < cloud->size(); ++i) {
    auto p = (*cloud)[i];
    const auto &c = refined_coeff.values;
    float d = std::abs(c[0] * p.x + c[1] * p.y + c[2] * p.z + c[3]);
    if (d < threshold)
      expected.push_back(static_cast<int>(i));
  }
  require(expected == refined.indices, "refined membership is stale");
  require(initial.indices != refined.indices,
          "fixture does not change membership");
  std::cout << "sac initial=";
  for (auto i : initial.indices)
    std::cout << i << ',';
  std::cout << " refined=";
  for (auto i : refined.indices)
    std::cout << i << ',';
  std::cout << " coefficients=";
  for (auto c : refined_coeff.values)
    std::cout << c << ',';
  std::cout << '\n';
}

int main() {
  try {
    require(regions(2, 0) == 2, "local-neighbour control failed");
    require(regions(3, 0) == 1, "coplanar distance bridge missing");
    require(regions(4, 20) == 1, "residual is unexpectedly a membership gate");
    morphology();
    membership();
    std::cout << "All segmentation contract checks passed\n";
  } catch (const std::exception &e) {
    std::cerr << e.what() << '\n';
    return 1;
  }
}
