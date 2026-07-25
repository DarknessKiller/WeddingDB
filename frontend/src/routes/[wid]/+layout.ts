import type { LayoutLoad } from './$types';
import { decodeId } from '$lib/utils/encode';

export const load: LayoutLoad = ({ params }) => {
  return {
    wid: decodeId(params.wid)
  };
};
