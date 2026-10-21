# frozen_string_literal: true

# Network sharing gateway formula for NetBridge (Binary Release).
class Netbridge < Formula
  desc 'User-space network sharing gateway for macOS'
  homepage 'https://github.com/fandiidnaf/netbridge'
  version '1.0.4'
  license 'MIT'

  if OS.mac?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-arm64.tar.gz"
      sha256 '5e459477d8a2b3827fd5e2122d64fc1192f7fbd1209213259690a12cd8dd07b3'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-darwin-amd64.tar.gz"
      sha256 'd2e00334e9b4f50df7ec9e98e8c29cce4d4661db1ddf11af947c0cd02b363e52'
    end
  elsif OS.linux?
    if Hardware::CPU.arm?
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-arm64.tar.gz"
      sha256 '3ef4cfbd5246fc50cafa53086608a7186bd361e74d12b5214bc893d7d9b56ecc'
    else
      url "https://github.com/fandiidnaf/netbridge/releases/download/v#{version}/netbridge-v#{version}-linux-amd64.tar.gz"
      sha256 '8cd1cd552f711305d24b9a24b5f2407871e7cf33cb5578f5e4c6c8c70d39da9f'
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
