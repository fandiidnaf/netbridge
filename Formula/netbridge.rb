# frozen_string_literal: true

# Network sharing gateway formula for NetBridge (Binary Release).
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  version '1.0.3'
  license 'MIT'

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-arm64.tar.gz"
      sha256 'b62dc38b03bd2b0bae255f730950c04b7494aca1596de0fe0e0a0ec82fe5e0b7'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-amd64.tar.gz"
      sha256 '58a1feb3646eccad0cf3eaa88bae734dc40d3987fcd6914ec70c411b09895be3'
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-arm64.tar.gz"
      sha256 'b2e1c9ec2b14a16b7f23df4e7d40742f52d4ca8e1e8b43363181fa00209e5263'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-amd64.tar.gz"
      sha256 'f87e8fd7384feb23681dc0d275f5d7e4a79ed3234e4c7521674748bc580a2405'
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
