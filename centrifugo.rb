class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.6/centrifugo-1.7.6-darwin-amd64.zip"
  sha256 "9ab90cd6f6255a9b1f96e162cf6f29d583e921d2afd2067350417fc4e9992deb"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
