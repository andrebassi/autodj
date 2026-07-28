/** @type {import('next').NextConfig} */
// Static export → web/out, served by the Go server (WEB_DIR) or Fly image.
const nextConfig = {
  output: "export",
  images: { unoptimized: true },
};
export default nextConfig;
