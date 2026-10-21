# frozen_string_literal: true

# Network sharing gateway formula for NetBridge.
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  version '1.0.0'
  license 'MIT'

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-arm64.tar.gz"
      sha256 'f1edd3a381524ddb40eb0896381744475bfc0b4ed7983b4d4ba4bc4f13ec85fc'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-amd64.tar.gz"
      sha256 'f09a2f096b36f5b4ce611ecb9563de6f67bf8b705b40d58d70fd755ed7db8e07'
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-arm64.tar.gz"
      sha256 '90aa0f9ce629a72ee6c6d2f1125abf0124535ba859a90fab2b0a808336a57b12'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-amd64.tar.gz"
      sha256 '845840c839b909f554780bebbd57c4a1d2aea73e604c0010c144d64f4b3bc39e'
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
