class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.8/centrifugo-1.7.8-darwin-amd64.zip"
  sha256 "ca4e7211635ade028c088fe018c5397540f16b6c0f26b8ceca185a1a1e1ca843"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
