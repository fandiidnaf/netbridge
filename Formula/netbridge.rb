# frozen_string_literal: true

# Network sharing gateway formula for NetBridge (Binary Release).
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  version '1.0.2'
  license 'MIT'

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-arm64.tar.gz"
      sha256 'ce1ee25a162f0d7df5985dea8aa2f0ab1001b26e44815a94ba8e2f3205802155'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-amd64.tar.gz"
      sha256 '42f243de7eb99874e74e0b95368428ae7195ac7712c343244e67477330c9d736'
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-arm64.tar.gz"
      sha256 '0aaabf9713d368339d36316ec36d899d525059b7c2f049d76a2a1ab78ae100b9'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-amd64.tar.gz"
      sha256 '54a603298d99483d67314e778e22fa5d1b387bb9f5984365aa200b48409d5678'
    end
  end

  def install
    binary_name = if OS.mac?
                    Hardware::CPU.arm? ? 'netbridge-darwin-arm64' : 'netbridge-darwin-amd64'
                  elsif Hardware::CPU.arm?
                    'netbridge-linux-arm64'
                  else
                    'netbridge-linux-amd64'
                  end

    bin.install binary_name => 'netbridge'
  end

  test do
    assert_match "netbridge v#{version}", shell_output("#{bin}/netbridge version")
  end
end
