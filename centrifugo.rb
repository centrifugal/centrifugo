class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.6.5/centrifugo-1.6.5-darwin-amd64.zip"
  sha256 "a8487e2c2f0947ad467e53725954c9784d6c1a8030a4003889e476af2186e4c3"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
