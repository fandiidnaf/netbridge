# frozen_string_literal: true

# Network sharing gateway formula for NetBridge.
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  url 'https://github.com/fandiidnaf/netbridge/archive/refs/tags/v1.0.0.tar.gz'
  sha256 '53b1d9a2d2e6479121e1cd9b6f6b00c9b08837457fd3f0480780e435f67b3e1a'
  license 'MIT'

  depends_on 'go' => :build

  def install
    system 'go', 'build', *std_go_args(ldflags: "-s -w -X main.version=#{version}"), './cmd/netbridge'
  end

  test do
    assert_match "netbridge v#{version}", shell_output("#{bin}/netbridge version")
  end
end
