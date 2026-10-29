// OQ-06 regression: public 2-D estimation suppresses z translation.
#include <cmath>
#include <iostream>
#include <pcl/point_types.h>
#include <pcl/registration/transformation_estimation_2D.h>

int main() {
  using Point = pcl::PointXYZ;
  pcl::PointCloud<Point> source, target;
  source.push_back(Point(0, 0, 1));
  source.push_back(Point(1, 0, 2));
  source.push_back(Point(0, 1, 3));
  for (const auto &p : source)
    target.push_back(Point(2 - p.y, -1 + p.x, p.z + 7));
  pcl::registration::TransformationEstimation2D<Point, Point> estimator;
  Eigen::Matrix4f actual;
  estimator.estimateRigidTransformation(source, target, actual);
  Eigen::Matrix4f expected = Eigen::Matrix4f::Identity();
  expected(0, 0) = expected(1, 1) = 0;
  expected(0, 1) = -1;
  expected(1, 0) = 1;
  expected(0, 3) = 2;
  expected(1, 3) = -1;
  if (!actual.isApprox(expected, 1e-5f)) {
    std::cerr << "unexpected public 2-D transform:\n" << actual << '\n';
    return 1;
  }
  // Exercise the correspondence overload that an iterative matcher would use.
  pcl::Correspondences pairs;
  for (int i = 0; i < 3; ++i)
    pairs.emplace_back(i, i, 0);
  estimator.estimateRigidTransformation(source, target, pairs, actual);
  if (!actual.isApprox(expected, 1e-5f))
    return 1;
  std::cout << "Both overloads: tx=2 ty=-1 yaw=90deg tz=0 despite target z "
               "offset 7\n";
}
