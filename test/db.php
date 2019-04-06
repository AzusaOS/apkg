<?php

// build data file + index
chdir(__DIR__.'/dist/data');

$outfp = fopen('db.bin', 'wb+');

fwrite($outfp, 'TPDB');
fwrite($outfp, pack('N', 0x00000001)); // version
fwrite($outfp, pack('J', 0)); // flags
fwrite($outfp, pack('JJ', time(), 0)); // creation date/time
fwrite($outfp, pack('N', 0)); // OS 0=linux 1=darwin 2=windows ...
fwrite($outfp, pack('N', 1)); // Arch 0=i386 1=amd64 ...

if (ftell($outfp) != 40) die("invalid header\n");

// at 40
fwrite($outfp, pack('N', 0)); // location of id index
fwrite($outfp, pack('N', 0)); // location of name index

$pkgs = [];

$it = new \RecursiveIteratorIterator(new \RecursiveDirectoryIterator('.'));
foreach($it as $fn => $info) {
	if (!$info->isFile()) continue;
	if (substr($fn, -5) != '.tpkg') continue;
	echo "Indexing: $fn\n";

	$fp = fopen($fn, 'rb');
	// read header
	$header_bin = fread($fp, 120);
	// decode
	$header = unpack('a4magic/Nversion/Jflags/Jcreation_time/Jcreation_time_us/Nmetadata_offt/Nmetadata_len/H64metadata_hash/Ntable_offt/Ntable_len/H64table_hash/Nsign_offt/Ndata_offt', $header_bin);

	// read metadata
	fseek($fp, $header['metadata_offt']);
	$metadata = fread($fp, $header['metadata_len']);
	// check checksum
	$hash = hash('sha256', $metadata, true);

	if ($hash != pack('H*', $header['metadata_hash'])) die("$fn: bad metadata hash\n");

	$metadata = json_decode($metadata, true);
	$header_hash = hash('sha256', $header_bin, true);
	$id = hash('md5', $metadata['full_name'], true);

	$pos = ftell($outfp);
	$pkgs[] = [
		'metadata' => $metadata,
		'id' => $id,
		'pos' => $pos,
	];

	fwrite($outfp, chr(0)); // node type = package
	fwrite($outfp, $id);
	fwrite($outfp, $header_hash);
	fwrite($outfp, pack('J', $info->getSize()));
	fwrite($outfp, chr(strlen($metadata['full_name'])).$metadata['full_name']); // TODO check len(full_name) < 256
}

// location of id index
$id_index_pos = ftell($outfp);
fseek($outfp, 40);
fwrite($outfp, pack('N', $id_index_pos));
fseek($outfp, $id_index_pos);

// let's create id index
// XXX TODO IN THE FUTURE


