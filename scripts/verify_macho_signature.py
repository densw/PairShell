"""Check Mach-O code-page and bundle-seal hashes, not Apple trust/notarization."""
import hashlib
import plistlib
import struct


def verify_binary(path, bundle=None):
    data = path.read_bytes()
    if data[:4] == bytes.fromhex('cafebabe'):
        count = struct.unpack_from('>I', data, 4)[0]
        slices = []
        for i in range(count):
            _, _, offset, size, _ = struct.unpack_from('>IIIII', data, 8 + 20*i)
            slices.append(data[offset:offset+size])
    else:
        slices = [data]
    for thin in slices:
        assert thin[:4] == bytes.fromhex('cffaedfe'), 'Expected 64-bit Mach-O'
        commands = struct.unpack_from('<I', thin, 16)[0]
        offset = 32
        signature = None
        for _ in range(commands):
            command, size = struct.unpack_from('<II', thin, offset)
            assert size >= 8
            if command == 0x1d:
                start, length = struct.unpack_from('<II', thin, offset+8)
                signature = thin[start:start+length]
            offset += size
        assert signature is not None, 'Missing code signature'
        magic, length, count = struct.unpack_from('>III', signature)
        assert magic == 0xfade0cc0 and length <= len(signature)
        blobs = {}
        for i in range(count):
            slot, pos = struct.unpack_from('>II', signature, 12+8*i)
            blob_length = struct.unpack_from('>I', signature, pos+4)[0]
            blobs[slot] = signature[pos:pos+blob_length]
        assert 0 in blobs, 'Missing code directory'
        for slot, cd in blobs.items():
            if slot != 0 and not 0x1000 <= slot < 0x1006:
                continue
            magic, _, version, flags, hashes, _, special, pages, limit = struct.unpack_from('>9I', cd)
            assert magic == 0xfade0c02 and flags & 2, 'Expected ad-hoc signature'
            hash_size, hash_type, _, page_shift = struct.unpack_from('4B', cd, 36)
            digest = {1: hashlib.sha1, 2: hashlib.sha256}[hash_type]
            assert hash_size == digest().digest_size
            if version >= 0x20300 and limit == 0xffffffff:
                limit = struct.unpack_from('>Q', cd, 56)[0]
            page_size = 1 << page_shift
            assert pages == (limit+page_size-1)//page_size
            for page in range(pages):
                actual = digest(thin[page*page_size:min((page+1)*page_size, limit)]).digest()
                assert actual == cd[hashes+page*hash_size:hashes+(page+1)*hash_size], 'Code-page hash mismatch'
            for seal in range(1, special+1):
                expected = cd[hashes-seal*hash_size:hashes-(seal-1)*hash_size]
                payload = blobs.get(seal)
                if bundle and seal == 1:
                    payload = (bundle/'Contents/Info.plist').read_bytes()
                if bundle and seal == 3:
                    payload = (bundle/'Contents/_CodeSignature/CodeResources').read_bytes()
                if payload is not None:
                    assert digest(payload).digest() == expected, 'Special-slot seal mismatch'
    if bundle:
        manifest = plistlib.loads((bundle/'Contents/_CodeSignature/CodeResources').read_bytes())
        for relative, entry in manifest.get('files2', {}).items():
            if isinstance(entry, dict) and 'hash2' in entry:
                assert hashlib.sha256((bundle/'Contents'/relative).read_bytes()).digest() == entry['hash2'], 'Resource hash mismatch'
    print('MACHO_HASHES_OK', path.name, len(slices), 'architectures')
