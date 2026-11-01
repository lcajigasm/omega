#!/usr/bin/env python3
"""Parser checks for scripts/verify_taps.py. No network."""

import unittest

from verify_taps import TapError, normalize, versions_in


class VersionsTest(unittest.TestCase):
    def test_url_versions_agree(self):
        text = """
        url "https://github.com/lcajigasm/omega.git", branch: "main"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_linux_amd64.tar.gz"
        """
        self.assertEqual(versions_in(text), "0.10.0")

    def test_explicit_version_must_match_urls(self):
        text = """
        version "0.10.0"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        """
        self.assertEqual(versions_in(text), "0.10.0")

    def test_mixed_versions_fail(self):
        text = """
        version "0.9.0"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        """
        with self.assertRaises(TapError):
            versions_in(text)

    def test_archive_name_must_match_url(self):
        text = 'url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.9.0_linux_amd64.tar.gz"'
        with self.assertRaises(TapError):
            versions_in(text)

    def test_head_url_alone_is_not_a_version(self):
        text = 'url "https://github.com/lcajigasm/omega.git", branch: "main"'
        with self.assertRaises(TapError):
            versions_in(text)

    def test_normalize_tag(self):
        self.assertEqual(normalize("v0.10.0"), "0.10.0")
        self.assertEqual(normalize(" 0.10.0 "), "0.10.0")


if __name__ == "__main__":
    unittest.main()
