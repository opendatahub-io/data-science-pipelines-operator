import os

import yaml


def table(rows):
    """
    Convert a list of cits into a markdown table.

    Pre-condition: All dicts in list_of_dicts should have identical key_sets
    :param rows: list of dict where each key set for every dict matches list of cols
    :return: A markdown where each row corresponds to a dict in list_of_dicts
    """

    markdown_table = ""
    if len(rows) == 0:
        return markdown_table

    cols = []
    for row in rows:
        cols.extend([key for key in row.keys() if key not in cols])

    markdown_header = '| ' + ' | '.join(cols) + ' |'
    markdown_header_separator = '|-----' * len(cols) + '|'
    markdown_table += markdown_header + '\n'
    markdown_table += markdown_header_separator + '\n'
    for row in rows:
        markdown_row = ""
        for col in cols:
            markdown_row += '| ' + str(row[col]) + ' '
        markdown_row += '|' + '\n'
        markdown_table += markdown_row
    return markdown_table


def version_doc(args):
    input_file = args.input_file
    out_file = args.out_file
    with open(input_file, 'r') as f:
        rows = yaml.safe_load(f)

    dirname = os.path.dirname(__file__)
    template_file = os.path.join(dirname, 'template/version_doc.md')
    with open(template_file, 'r') as vd:
        final_md = vd.read()

    table_md = table(rows)

    final_md = final_md.replace('<<GENERATED_COMPATIBILITY_TABLE>>', table_md)
    final_md = '<!--THIS DOC IS AUTO GENERATED-->\n' + final_md

    with open(out_file, 'w') as f:
        f.write(final_md)
