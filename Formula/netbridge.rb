# frozen_string_literal: true

# Network sharing gateway formula for NetBridge (Binary Release).
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  version '1.0.1'
  license 'MIT'

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-arm64.tar.gz"
      sha256 '43bdb3a9378bf8f5670a941b08aa44d7df3dc0422e7dab41b811ae93d2eedac1'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-amd64.tar.gz"
      sha256 '15b7c6045d01cd3758dbcce56741d3846b7df3df3672b5a796a8b0f1cf253740'
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-arm64.tar.gz"
      sha256 '079a145f74526589ed0bd57a1c4f897b74738c3fce93dc5dad13b5dcbe57f5d3'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-amd64.tar.gz"
      sha256 'b9600be21ac0eb19fb273fbaf9269935ef89366bb8db3ca2ec431848bb3d0655'
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
