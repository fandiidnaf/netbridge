class Netbridge < Formula
  desc "User-space network sharing gateway for macOS"
  homepage "https://github.com/fandiidnaf/netbridge"
  url "https://github.com/fandiidnaf/netbridge/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/netbridge"
  end

  test do
    assert_match "netbridge v#{version}", shell_output("#{bin}/netbridge version")
  end
end
